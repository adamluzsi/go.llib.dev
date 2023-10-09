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
)

func main() {
	projects, err := getProjects()
	if err != nil {
		log.Fatalln(err.Error())
	}

	if err := generateProjectRedirects(projects); err != nil {
		log.Fatalln(err.Error())
	}
}

func generateProjectRedirects(projects []string) error {
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
			dirPath = filepath.Join(outDirPath, project)
			outPath = filepath.Join(dirPath, "index.html")
		)

		if err := tmpl.Execute(&buf, RedirectTemplateData{Project: project}); err != nil {
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
	Project string
}

func getProjects() ([]string, error) {
	// Read environment variable
	filePath, ok := os.LookupEnv("PROJECTS_FILE_PATH")
	if !ok {
		return nil,
			fmt.Errorf("PROJECTS_FILE_PATH environment variable is not set")
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil,
			fmt.Errorf("failed to open projects file: %w", err)
	}
	defer file.Close()

	// Create a new scanner and read the file line by line
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	var projects []string
	projects = []string{}

	for scanner.Scan() {
		projects = append(projects, scanner.Text())
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
