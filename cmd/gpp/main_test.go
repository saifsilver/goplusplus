package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLIScaffoldApp(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gpp_test_app_*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	appName := filepath.Join(tempDir, "sampleapp")
	if err := scaffoldApp(scaffoldOptions{Directory: appName, ModulePath: "example.com/acme/sampleapp"}); err != nil {
		t.Fatalf("scaffold app: %v", err)
	}

	expectedFiles := []string{
		filepath.Join(appName, "go.mod"),
		filepath.Join(appName, "cmd", "api", "main.go"),
		filepath.Join(appName, "internal", "application", "application.go"),
		filepath.Join(appName, "internal", "config", "config.go"),
		filepath.Join(appName, "internal", "modules", "system", "module.go"),
		filepath.Join(appName, "internal", "modules", "users", "module.go"),
		filepath.Join(appName, "internal", "modules", "users", "service.go"),
		filepath.Join(appName, "internal", "modules", "users", "repository.go"),
		filepath.Join(appName, "internal", "modules", "users", "migrations", "0001_users.sql"),
		filepath.Join(appName, "Makefile"),
		filepath.Join(appName, "Dockerfile"),
	}

	for _, file := range expectedFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("expected generated file '%s' to exist", file)
		}
	}

	frameworkRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile(t)), "..", ".."))
	goModPath := filepath.Join(appName, "go.mod")
	goMod, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read generated go.mod: %v", err)
	}
	goMod = append(goMod, []byte("\nreplace github.com/saifsilver/goplusplus => "+frameworkRoot+"\n")...)
	if err := os.WriteFile(goModPath, goMod, 0o644); err != nil {
		t.Fatalf("configure local framework replacement: %v", err)
	}

	command := exec.Command("go", "test", "-mod=mod", "./...")
	command.Dir = appName
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated application does not pass its tests: %v\n%s", err, output)
	}
}

func TestScaffoldRefusesExistingDirectory(t *testing.T) {
	directory := t.TempDir()
	err := scaffoldApp(scaffoldOptions{Directory: directory, ModulePath: "example.com/existing"})
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}
}

func TestParseScaffoldOptions(t *testing.T) {
	options, err := parseScaffoldOptions([]string{"payments", "--module", "example.com/acme/payments"})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if options.Directory != "payments" || options.ModulePath != "example.com/acme/payments" {
		t.Fatalf("unexpected options: %#v", options)
	}
	if _, err := parseScaffoldOptions([]string{"payments", "--module", "../invalid"}); err == nil {
		t.Fatal("expected invalid module path to be rejected")
	}
}

func TestRunCLIReturnsCommandErrors(t *testing.T) {
	if err := runCLI([]string{"new"}); err == nil {
		t.Fatal("expected invalid new command to return an error")
	}
	if err := runCLI([]string{"gen", "unknown", "item"}); err == nil {
		t.Fatal("expected unknown generator to return an error")
	}
	if err := runCLI([]string{"gen", "module", "../escape"}); err == nil {
		t.Fatal("expected unsafe generator name to return an error")
	}
	if err := runCLI([]string{"extract", "service", "users"}); err == nil {
		t.Fatal("expected missing extraction module path to return an error")
	}
}

func TestExtractServiceEndToEnd(t *testing.T) {
	root := t.TempDir()
	applicationRoot := filepath.Join(root, "source-app")
	if err := scaffoldApp(scaffoldOptions{
		Directory: applicationRoot, ModulePath: "example.com/acme/source-app",
	}); err != nil {
		t.Fatalf("scaffold source app: %v", err)
	}

	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(applicationRoot); err != nil {
		t.Fatalf("enter source app: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })

	if err := generateModule("orders"); err != nil {
		t.Fatalf("generate portable module: %v", err)
	}
	if err := runCLI([]string{
		"extract", "service", "orders",
		"--module", "example.com/acme/orders-service",
		"--route", "/api/v1/orders",
	}); err != nil {
		t.Fatalf("extract service: %v", err)
	}

	serviceRoot := filepath.Join(applicationRoot, "services", "orders")
	assertGeneratedFiles(t, serviceRoot, []string{
		"go.mod", "Dockerfile", "MIGRATION.md", "cmd/api/main.go",
		"internal/application/application.go", "internal/modules/orders/module.go",
		"internal/modules/orders/migrations/0001_create_records.sql",
	})
	assertFileContains(t, filepath.Join(serviceRoot, "internal", "application", "application.go"),
		`capability "example.com/acme/orders-service/internal/modules/orders"`,
		`engine.RegisterModule("/api/v1/orders", capability.Build(database))`)
	assertFileContains(t, filepath.Join(serviceRoot, "internal", "modules", "orders", "migrations.go"),
		"Version: 2")

	frameworkRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile(t)), "..", ".."))
	goModPath := filepath.Join(serviceRoot, "go.mod")
	goMod, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read extracted go.mod: %v", err)
	}
	goMod = append(goMod, []byte("\nreplace github.com/saifsilver/goplusplus => "+frameworkRoot+"\n")...)
	if err := os.WriteFile(goModPath, goMod, 0o644); err != nil {
		t.Fatalf("configure local framework replacement: %v", err)
	}
	command := exec.Command("go", "test", "-mod=mod", "./...")
	command.Dir = serviceRoot
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("extracted service does not pass its tests: %v\n%s", err, output)
	}
	if err := runCLI([]string{
		"extract", "service", "users",
		"--module", "example.com/acme/users-service",
	}); err != nil {
		t.Fatalf("extract scaffolded users module: %v", err)
	}
	assertGeneratedFiles(t, filepath.Join(applicationRoot, "services", "users"), []string{
		"go.mod", "internal/modules/users/module.go", "internal/modules/users/migrations/0001_users.sql",
	})

	hiddenDependency := []byte("package orders\n\nimport _ \"example.com/acme/source-app/internal/config\"\n")
	if err := os.WriteFile(filepath.Join(applicationRoot, "internal", "modules", "orders", "hidden.go"), hiddenDependency, 0o644); err != nil {
		t.Fatalf("write hidden dependency fixture: %v", err)
	}
	err = runCLI([]string{
		"extract", "service", "orders",
		"--module", "example.com/acme/invalid-service",
		"--output", "services/invalid-orders",
	})
	if err == nil || !strings.Contains(err.Error(), "hidden dependency") {
		t.Fatalf("expected cross-module dependency rejection, got %v", err)
	}
}

func TestDeploymentGenerators(t *testing.T) {
	project := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":     "module example.com/deployment\n\ngo 1.26.5\n",
		"Dockerfile": "FROM scratch\n",
	} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write project marker: %v", err)
		}
	}

	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatalf("enter test project: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })

	if err := runCLI([]string{"gen", "terraform", "aws"}); err != nil {
		t.Fatalf("generate Terraform: %v", err)
	}
	terraformFiles := []string{
		"versions.tf", "networking.tf", "database.tf", "service.tf", "outputs.tf",
		"terraform.tfvars.example", "backend.tf.example", "deploy.sh", "README.md",
	}
	assertGeneratedFiles(t, filepath.Join(project, "deploy", "terraform", "aws"), terraformFiles)
	assertFileContains(t, filepath.Join(project, "deploy", "terraform", "aws", "database.tf"),
		"manage_master_user_password = true", "publicly_accessible    = false", "storage_encrypted")
	assertFileContains(t, filepath.Join(project, "deploy", "terraform", "aws", "service.tf"),
		"deployment_circuit_breaker", "aws_appautoscaling_target", "DB_PASSWORD", "assign_public_ip = false")
	assertFileContains(t, filepath.Join(project, "deploy", "terraform", "aws", "deploy.sh"),
		"docker build --pull", "../../..")
	if err := runCLI([]string{"gen", "terraform", "aws"}); err == nil {
		t.Fatal("expected Terraform overwrite refusal")
	}

	if err := runCLI([]string{"gen", "hosting", "standard"}); err != nil {
		t.Fatalf("generate standard hosting: %v", err)
	}
	standardRoot := filepath.Join(project, "deploy", "standard")
	assertGeneratedFiles(t, standardRoot, []string{
		"compose.yaml", "Caddyfile", ".env.example", "backup.sh", "README.md",
	})
	assertFileContains(t, filepath.Join(standardRoot, "compose.yaml"),
		"condition: service_healthy", "DB_PASSWORD_FILE", "internal: true", "read_only: true")
	assertFileContains(t, filepath.Join(standardRoot, "Caddyfile"),
		"reverse_proxy app:8080", "health_uri /health/ready")
	validateGeneratedDeploymentTools(t, filepath.Join(project, "deploy", "terraform", "aws"), standardRoot)
}

func TestDeploymentGeneratorRequiresProjectRoot(t *testing.T) {
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("enter empty directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })
	if err := generateTerraform("aws"); err == nil {
		t.Fatal("expected missing project markers to be rejected")
	}
	if err := generateTerraform("gcp"); err == nil {
		t.Fatal("expected unsupported Terraform provider to be rejected")
	}
}

func assertGeneratedFiles(t *testing.T, root string, names []string) {
	t.Helper()
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("expected generated file %s: %v", name, err)
		}
	}
}

func assertFileContains(t *testing.T, path string, values ...string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, value := range values {
		if !strings.Contains(string(content), value) {
			t.Errorf("expected %s to contain %q", path, value)
		}
	}
}

func validateGeneratedDeploymentTools(t *testing.T, terraformRoot, standardRoot string) {
	t.Helper()
	if terraformBinary, err := exec.LookPath("terraform"); err == nil {
		command := exec.Command(terraformBinary, "fmt", "-check", "-recursive")
		command.Dir = terraformRoot
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("generated Terraform formatting failed: %v\n%s", err, output)
		}
	}
	if dockerBinary, err := exec.LookPath("docker"); err == nil {
		secretPath := filepath.Join(standardRoot, "secrets", "postgres_password")
		if err := os.WriteFile(secretPath, []byte("test-secret\n"), 0o600); err != nil {
			t.Fatalf("write Compose test secret: %v", err)
		}
		command := exec.Command(dockerBinary, "compose", "--env-file", ".env.example", "config", "--quiet")
		command.Dir = standardRoot
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("generated Compose validation failed: %v\n%s", err, output)
		}
	}
}

func currentFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	return file
}

func TestCLIGenerators(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gpp_test_gen_*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer func() { _ = os.Chdir(origWd) }()

	if err := generateModule("order"); err != nil {
		t.Fatalf("generate module: %v", err)
	}
	if _, err := os.Stat(filepath.Join("internal", "modules", "order", "module.go")); os.IsNotExist(err) {
		t.Errorf("generateModule failed to create internal/modules/order/module.go")
	}

	generateMiddleware("auth_header")
	if _, err := os.Stat(filepath.Join("middleware", "auth_header.go")); os.IsNotExist(err) {
		t.Errorf("generateMiddleware failed to create middleware/auth_header.go")
	}

	generateMigration("create_orders")
	entries, _ := os.ReadDir("migrations")
	if len(entries) == 0 {
		t.Errorf("generateMigration failed to create SQL file in migrations/")
	}

	generateHandler("user_profile")
	if _, err := os.Stat(filepath.Join("handlers", "user_profile.go")); os.IsNotExist(err) {
		t.Errorf("generateHandler failed to create handlers/user_profile.go")
	}
}

func TestInteractiveCLI(t *testing.T) {
	tempDir := t.TempDir()
	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer func() { _ = os.Chdir(origWd) }()

	// Test Choice 9 (Version)
	input := strings.NewReader("9\n")
	var output strings.Builder
	if err := runInteractiveCLI(input, &output); err != nil {
		t.Fatalf("runInteractiveCLI version: %v", err)
	}
	if !strings.Contains(output.String(), "goplusplus (gpp) CLI Tool") {
		t.Errorf("expected version output, got %q", output.String())
	}

	// Test Choice 10 (Exit)
	inputExit := strings.NewReader("10\n")
	var outputExit strings.Builder
	if err := runInteractiveCLI(inputExit, &outputExit); err != nil {
		t.Fatalf("runInteractiveCLI exit: %v", err)
	}
	if !strings.Contains(outputExit.String(), "Goodbye!") {
		t.Errorf("expected exit message, got %q", outputExit.String())
	}

	// Test Choice 2 (Generate Module)
	inputGen := strings.NewReader("2\ninvoices\n")
	var outputGen strings.Builder
	if err := runInteractiveCLI(inputGen, &outputGen); err != nil {
		t.Fatalf("runInteractiveCLI generate module: %v", err)
	}
	if _, err := os.Stat(filepath.Join("internal", "modules", "invoices", "module.go")); os.IsNotExist(err) {
		t.Errorf("interactive generator failed to create internal/modules/invoices/module.go")
	}
}
