//go:generate go run .
package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	imports, err := getImports()
	if err != nil {
		log.Fatalln(err.Error())
	}

	if err := generateProjectRedirects(imports); err != nil {
		log.Fatalln(err.Error())
	}
}

func generateProjectRedirects(projects []Import) error {
	const outDirEnvKey = "WEB_DIR_PATH"

	outDirPath, ok := os.LookupEnv(outDirEnvKey)
	if !ok {
		return fmt.Errorf("%s env variable not set", outDirEnvKey)
	}

	tmpl, err := getRedirectTemplate()
	if err != nil {
		return fmt.Errorf("getRedirectTemplate failed: %w", err)
	}

	for _, project := range projects {
		var (
			buf     bytes.Buffer
			dirPath = filepath.Join(outDirPath, project.Name)
			outPath = filepath.Join(dirPath, "index.html")
		)

		if err := tmpl.Execute(&buf, RedirectTemplateData{
			PackageName: project.Name,
			ImportDef:   project.Import,
		}); err != nil {
			return fmt.Errorf("redirect template execution failed: %w", err)
		}
		if err := ensureDirectory(dirPath); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("writing out html failed: %w", err)
		}

		log.Println("INFO", fmt.Sprintf("%s redirect is created", project))
	}

	return nil
}

//go:embed redirect.html
var redirectRaw string

// getRedirectTemplate is the Go import redirect template
func getRedirectTemplate() (*template.Template, error) {
	return template.New("go-redirect").Parse(redirectRaw)
}

type RedirectTemplateData struct {
	PackageName string
	ImportDef   string
}

type Import struct {
	Name   string
	Import string
}

func getImports() ([]Import, error) {
	const envKey = "IMPORTS_FILE_PATH"
	// Read environment variable
	filePath, ok := os.LookupEnv(envKey)
	if !ok {
		return nil,
			fmt.Errorf("%s environment variable is not set", envKey)
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil,
			fmt.Errorf("failed to open imports file: %w", err)
	}
	defer file.Close()

	// Create a new scanner and read the file line by line
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	var projects []Import

	var get = func(e []string, i int) string {
		if i < len(e) {
			return e[i]
		}
		return ""
	}

	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}

		parts := strings.Split(raw, ";")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}

		if n := len(parts); n == 0 || 2 < n {
			return nil, fmt.Errorf("project value is not interpretable: %s", raw)
		}

		projects = append(projects, Import{
			Name:   get(parts, 0),
			Import: get(parts, 1),
		})
	}

	// Check for errors during scanning
	if err := scanner.Err(); err != nil {
		return nil,
			fmt.Errorf("failed to read file: %w", err)
	}

	return projects, nil
}

// ensureDirectory attempts to create a directory at the specified path.
// It returns nil if the directory was created successfully or already exists,
// and an error if any occurred.
func ensureDirectory(path string) error {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return err
	}
	return nil
}
