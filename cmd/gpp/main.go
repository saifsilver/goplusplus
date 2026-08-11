package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]
	switch command {
	case "new":
		if len(os.Args) < 3 {
			fmt.Println("❌ Error: Please provide an app name. Usage: gpp new <app_name>")
			return
		}
		scaffoldApp(os.Args[2])
	case "gen":
		if len(os.Args) < 4 || os.Args[2] != "module" {
			fmt.Println("❌ Error: Usage: gpp gen module <module_name>")
			return
		}
		generateModule(os.Args[3])
	case "version":
		fmt.Println("goplusplus (gpp) CLI v1.0.0")
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("🚀 goplusplus (gpp) CLI Tool v1.5.0")
	fmt.Println("Usage:")
	fmt.Println("  gpp new <app_name>     - Scaffold a production-ready goplusplus application")
	fmt.Println("  gpp gen module <name>  - Generate a new domain module in modules/<name>")
	fmt.Println("  gpp migrate            - Run database migrations on binary app")
	fmt.Println("  gpp seed               - Seed database with fake data")
	fmt.Println("  gpp version            - Display framework version")
}

func scaffoldApp(appName string) {
	fmt.Printf("🚀 Scaffolding production-ready goplusplus app '%s'...\n", appName)
	_ = os.MkdirAll(appName, 0755)

	mainContent := fmt.Sprintf(`package main

import (
	"fmt"
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func main() {
	app := gpp.New()

	app.Use(
		middleware.Observability(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		middleware.CORS(),
	)

	v1 := app.Group("/api/v1")
	v1.GET("/health", func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{
			"status": "UP",
			"app":    "%s",
		})
	})

	fmt.Println("🚀 Server running on http://localhost:8080")
	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
`, appName)

	_ = os.WriteFile(filepath.Join(appName, "main.go"), []byte(mainContent), 0644)
	fmt.Printf("✅ Project '%s' created successfully! Run 'cd %s && go run main.go'\n", appName, appName)
}

func generateModule(moduleName string) {
	dir := filepath.Join("modules", strings.ToLower(moduleName))
	fmt.Printf("🧱 Generating domain module '%s' in %s/...\n", moduleName, dir)
	_ = os.MkdirAll(dir, 0755)

	titleName := strings.Title(moduleName)
	content := fmt.Sprintf(`package %s

import (
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
)

type Module struct{}

func (m *Module) Name() string { return "%sModule" }

func (m *Module) Register(group *gpp.RouterGroup) {
	group.GET("/status", m.getStatus)
}

func (m *Module) getStatus(c *gpp.Context) error {
	return c.JSON(http.StatusOK, gpp.H{
		"module": "%s",
		"status": "active",
	})
}
`, strings.ToLower(moduleName), titleName, titleName)

	_ = os.WriteFile(filepath.Join(dir, "module.go"), []byte(content), 0644)
	fmt.Printf("✅ Domain module '%s' generated successfully!\n", titleName)
}
