package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var deploymentNamePattern = regexp.MustCompile(`[^a-z0-9-]+`)

func generateTerraform(provider string) error {
	if strings.ToLower(provider) != "aws" {
		return fmt.Errorf("unsupported Terraform provider %q; supported provider: aws", provider)
	}
	projectName, err := deploymentProjectName()
	if err != nil {
		return err
	}
	return publishGeneratedTree(
		filepath.Join("deploy", "terraform", "aws"),
		"Terraform deployment",
		deploymentFiles(projectName, awsTerraformFiles()),
	)
}

func generateHosting(target string) error {
	if strings.ToLower(target) != "standard" {
		return fmt.Errorf("unsupported hosting target %q; supported target: standard", target)
	}
	projectName, err := deploymentProjectName()
	if err != nil {
		return err
	}
	return publishGeneratedTree(
		filepath.Join("deploy", "standard"),
		"Standard hosting deployment",
		deploymentFiles(projectName, standardHostingFiles()),
	)
}

func deploymentProjectName() (string, error) {
	for _, required := range []string{"go.mod", "Dockerfile"} {
		if info, err := os.Stat(required); err != nil || info.IsDir() {
			return "", fmt.Errorf("%s is required; run the deployment generator from a goplusplus application root", required)
		}
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}
	return normalizeDeploymentName(filepath.Base(workingDirectory)), nil
}

func normalizeDeploymentName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = deploymentNamePattern.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		name = "gpp-app"
	}
	if name[0] < 'a' || name[0] > 'z' {
		name = "app-" + name
	}
	if len(name) < 3 {
		name += "-app"
	}
	if len(name) > 24 {
		name = strings.TrimRight(name[:24], "-")
	}
	return name
}

func deploymentFiles(projectName string, files []scaffoldFile) []scaffoldFile {
	replacer := strings.NewReplacer("{{APP_NAME}}", projectName)
	result := make([]scaffoldFile, 0, len(files))
	for _, file := range files {
		file.content = replacer.Replace(file.content)
		result = append(result, file)
	}
	return result
}

func publishGeneratedTree(target, description string, files []scaffoldFile) (err error) {
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("deployment directory %q already exists; refusing to overwrite it", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect deployment directory: %w", err)
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create deployment parent: %w", err)
	}
	temporary, err := os.MkdirTemp(parent, ".gpp-deploy-")
	if err != nil {
		return fmt.Errorf("create temporary deployment: %w", err)
	}
	defer func() {
		if temporary != "" {
			err = errors.Join(err, os.RemoveAll(temporary))
		}
	}()

	for _, file := range files {
		path := filepath.Join(temporary, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", file.path, err)
		}
		if err := os.WriteFile(path, []byte(file.content), file.mode); err != nil {
			return fmt.Errorf("write %s: %w", file.path, err)
		}
	}
	if err := os.Rename(temporary, target); err != nil {
		return fmt.Errorf("publish deployment: %w", err)
	}
	temporary = ""
	fmt.Printf("✅ %s created in %s.\n", description, target)
	return nil
}
