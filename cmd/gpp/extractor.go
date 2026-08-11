package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	gpp "github.com/saifsilver/goplusplus"
)

const maximumExtractedFileSize = 10 << 20

type extractOptions struct {
	Capability string
	ModulePath string
	Output     string
	Route      string
}

func parseExtractOptions(args []string) (extractOptions, error) {
	if len(args) < 2 || strings.ToLower(args[0]) != "service" {
		return extractOptions{}, errors.New("usage: gpp extract service <module> --module <go-module-path> [--output <dir>] [--route <path>]")
	}
	capability := strings.ToLower(args[1])
	if !generatorNamePattern.MatchString(capability) {
		return extractOptions{}, fmt.Errorf("module name %q must be a Go identifier", args[1])
	}

	flags := flag.NewFlagSet("extract service", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	modulePath := flags.String("module", "", "Go module path for the extracted service")
	output := flags.String("output", filepath.Join("services", capability), "output directory")
	route := flags.String("route", "/api/v1/"+capability, "HTTP route prefix")
	if err := flags.Parse(args[2:]); err != nil {
		return extractOptions{}, fmt.Errorf("parse extract command: %w", err)
	}
	if flags.NArg() != 0 || *modulePath == "" {
		return extractOptions{}, errors.New("usage: gpp extract service <module> --module <go-module-path> [--output <dir>] [--route <path>]")
	}
	if err := validateModulePath(*modulePath); err != nil {
		return extractOptions{}, err
	}
	cleanOutput := filepath.Clean(strings.TrimSpace(*output))
	if cleanOutput == "." || cleanOutput == string(filepath.Separator) {
		return extractOptions{}, errors.New("output must identify a new directory")
	}
	cleanRoute := path.Clean(strings.TrimSpace(*route))
	if !strings.HasPrefix(cleanRoute, "/") || cleanRoute == "/" {
		return extractOptions{}, errors.New("route must be a non-root absolute HTTP path")
	}
	return extractOptions{
		Capability: capability,
		ModulePath: *modulePath,
		Output:     cleanOutput,
		Route:      cleanRoute,
	}, nil
}

func extractService(options extractOptions) error {
	sourceModule, err := readGoModulePath("go.mod")
	if err != nil {
		return fmt.Errorf("read source module: %w", err)
	}
	sourceDirectory := filepath.Join("internal", "modules", options.Capability)
	info, err := os.Stat(sourceDirectory)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("module %q was not found at %s", options.Capability, sourceDirectory)
	}

	moduleFiles, err := collectPortableModule(sourceDirectory, sourceModule, options)
	if err != nil {
		return err
	}
	files := append(extractedServiceFiles(options), moduleFiles...)
	if err := publishGeneratedTree(options.Output, "Extracted HTTP microservice", files); err != nil {
		return err
	}
	fmt.Printf("👉 Next: cd %s && go mod tidy && make test\n", options.Output)
	fmt.Println("⚠️  The monolith was not modified; cut traffic over only after data and client migration are complete.")
	return nil
}

func readGoModulePath(goModPath string) (string, error) {
	file, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("go.mod has no module directive")
}

func collectPortableModule(sourceDirectory, sourceModule string, options extractOptions) ([]scaffoldFile, error) {
	oldImportPrefix := sourceModule + "/internal/modules/" + options.Capability
	newImportPrefix := options.ModulePath + "/internal/modules/" + options.Capability
	var files []scaffoldFile
	var buildFound, migrationsFound bool

	err := filepath.WalkDir(sourceDirectory, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("module contains unsupported symbolic link %s", filePath)
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".terraform" {
				return fmt.Errorf("module contains unsupported tool state directory %s", filePath)
			}
			return nil
		}
		if isSensitiveModuleFile(entry.Name()) {
			return fmt.Errorf("module contains potentially sensitive file %s; move secrets to runtime configuration", filePath)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > maximumExtractedFileSize {
			return fmt.Errorf("module file %s must be regular and no larger than 10 MiB", filePath)
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(sourceDirectory, filePath)
		if err != nil {
			return err
		}
		if filepath.Ext(filePath) == ".go" {
			rootFile := filepath.Dir(relative) == "."
			foundBuild, foundMigrations, err := auditGoFile(filePath, content, sourceModule, oldImportPrefix, rootFile, options.Capability)
			if err != nil {
				return err
			}
			buildFound = buildFound || foundBuild
			migrationsFound = migrationsFound || foundMigrations
			content = bytes.ReplaceAll(content, []byte(oldImportPrefix), []byte(newImportPrefix))
		}
		files = append(files, scaffoldFile{
			path:    filepath.Join("internal", "modules", options.Capability, relative),
			content: string(content),
			mode:    info.Mode().Perm(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("audit module %q: %w", options.Capability, err)
	}
	if !buildFound || !migrationsFound {
		return nil, fmt.Errorf(
			"module %q is not portable: add Build(*dbcore.Client) and Migrations() before extraction",
			options.Capability,
		)
	}
	return files, nil
}

func isSensitiveModuleFile(name string) bool {
	name = strings.ToLower(name)
	return name == ".env" || name == "credentials" ||
		strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".key")
}

func auditGoFile(filePath string, content []byte, sourceModule, ownPrefix string, rootFile bool, capability string) (bool, bool, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), filePath, content, parser.SkipObjectResolution)
	if err != nil {
		return false, false, fmt.Errorf("parse %s: %w", filePath, err)
	}
	if rootFile && parsed.Name.Name != capability {
		return false, false, fmt.Errorf("root package must be %q, found %q in %s", capability, parsed.Name.Name, filePath)
	}
	for _, imported := range parsed.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return false, false, fmt.Errorf("parse import in %s: %w", filePath, err)
		}
		if hasImportPrefix(importPath, sourceModule+"/internal") && !hasImportPrefix(importPath, ownPrefix) {
			return false, false, fmt.Errorf("hidden dependency %q in %s crosses the module boundary", importPath, filePath)
		}
	}
	var buildFound, migrationsFound bool
	if rootFile {
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil {
				continue
			}
			buildFound = buildFound || function.Name.Name == "Build"
			migrationsFound = migrationsFound || function.Name.Name == "Migrations"
		}
	}
	return buildFound, migrationsFound, nil
}

func hasImportPrefix(importPath, prefix string) bool {
	return importPath == prefix || strings.HasPrefix(importPath, prefix+"/")
}

func extractedServiceFiles(options extractOptions) []scaffoldFile {
	replacer := strings.NewReplacer(
		"{{MODULE}}", options.ModulePath,
		"{{APP_NAME}}", options.Capability+"-service",
		"{{CAPABILITY}}", options.Capability,
		"{{ROUTE}}", options.Route,
		"{{FRAMEWORK_VERSION}}", gpp.Version,
	)
	file := func(filePath, content string) scaffoldFile {
		return scaffoldFile{path: filePath, content: replacer.Replace(content), mode: 0o644}
	}
	return []scaffoldFile{
		file("go.mod", scaffoldGoMod),
		file("cmd/api/main.go", scaffoldMain),
		file("internal/application/application.go", extractedApplication),
		file("internal/application/application_test.go", extractedApplicationTest),
		file("internal/config/config.go", scaffoldConfig),
		file("internal/config/config_test.go", scaffoldConfigTest),
		file("internal/modules/system/module.go", scaffoldSystemModule),
		file("data/.gitkeep", ""),
		file(".env.example", scaffoldEnv),
		file(".gitignore", scaffoldGitignore),
		file(".dockerignore", scaffoldDockerignore),
		file("Dockerfile", scaffoldDockerfile),
		file("Makefile", extractedMakefile),
		file("README.md", extractedReadme),
		file("MIGRATION.md", extractedMigrationGuide),
	}
}
