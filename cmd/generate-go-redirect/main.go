//go:generate go run .
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/zerokit"
)

func main() {
	ctx := context.Background()
	if err := Main(ctx); err != nil {
		logger.Fatal(ctx, "error in main", logging.ErrField(err))
		os.Exit(1)
	}
}

func Main(ctx context.Context) error {
	metas, err := getMetas()
	if err != nil {
		return fmt.Errorf("get import meta data failed: %w", err)
	}
	if err := generateProjectRedirects(ctx, metas); err != nil {
		return fmt.Errorf("generate project redirects have failed: %w", err)
	}
	return nil
}

func generateProjectRedirects(ctx context.Context, metas []Meta) error {
	const outDirEnvKey = "WEB_DIR_PATH"

	domain, found, err := env.Lookup[string]("DOMAIN")
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("missing DOMAIN env variable")
	}

	outDirPath, ok := os.LookupEnv(outDirEnvKey)
	if !ok {
		return fmt.Errorf("%s env variable not set", outDirEnvKey)
	}

	tmpl, err := getImportTemplate()
	if err != nil {
		return fmt.Errorf("getRedirectTemplate failed: %w", err)
	}

	if err := os.WriteFile(filepath.Join(outDirPath, "CNAME"), []byte(domain), 0666); err != nil {
		return err
	}

	for _, meta := range metas {
		if !strings.Contains(meta.Import.Prefix, domain) {
			continue
		}

		var (
			buf     bytes.Buffer
			impPath = strings.TrimPrefix(meta.Import.Prefix, domain+"/")
			dirPath = filepath.Join(outDirPath, impPath)
			outPath = filepath.Join(dirPath, "index.html")
		)

		if err := tmpl.Execute(&buf, meta); err != nil {
			return fmt.Errorf("redirect template execution failed: %w", err)
		}
		if err := ensureDirectory(dirPath); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("writing out html failed: %w", err)
		}

		logger.Info(ctx, "redirection created", logging.Field("import", meta.Import.Prefix))

		for _, mod := range meta.Nested {
			var (
				modDirPath = filepath.Join(outDirPath, impPath, filepath.Join(path.Split(mod.Path)))
				modOutPath = filepath.Join(modDirPath, "index.html")
			)
			if err := ensureDirectory(modDirPath); err != nil {
				return err
			}
			// in Go, the nested module should have a go-import that tag points to the root project
			// hence we write the same content there as to its parent
			if err := os.WriteFile(modOutPath, buf.Bytes(), 0644); err != nil {
				return fmt.Errorf("writing %s nested module's out html failed: %w", mod.Path, err)
			}

			logger.Info(ctx, "nested module's redirection created", logging.Field("import", path.Join(meta.Import.Prefix, mod.Path)))
		}

	}

	return nil
}

//go:embed go-import.html
var goImportHTML string

// getImportTemplate is the Go import redirect template
func getImportTemplate() (*template.Template, error) {
	return template.New("go-redirect").Parse(goImportHTML)
}

type Meta struct {
	Import MetaImport
	Source MetaSource
	Nested []MetaNestedModule
}

type MetaImport struct {
	Prefix string
	VCS    MetaImportVCS
}

type MetaImportVCS struct {
	Name     string `enum:"git,"`
	RepoRoot *url.URL
}

type MetaNestedModule struct {
	Path string
}

// MetaSource
//
// {dir} - The import path with prefix and leading "/" trimmed.
// {/dir} - If {dir} is not the empty string, then {/dir} is replaced by "/" +
// {dir}. Otherwise, {/dir} is replaced with the empty string.
//
// {file} - The name of the file
// {line} - The decimal line number.
type MetaSource struct {
	// HomepageURL is the home URL that the source uses
	//
	// default: _
	HomepageURL string
	// Directory is the directory pattern it should use
	DirectoryPattern string
	// File is the file pattern that the go import should use
	FilePattern string
}

// var findURL = regexp.MustCompile(`https?://[^\s+]+`)

func getMetas() ([]Meta, error) {
	type JSONDTO struct {
		VCS              string   `json:"vcs"`
		ImportPrefix     string   `json:"import-prefix"`
		RootRepo         string   `json:"root-repo"`
		HomepageURL      string   `json:"homepage"`
		DirectoryPattern string   `json:"directory-pattern"`
		FilePattern      string   `json:"file-pattern"`
		Submodules       []string `json:"submodules"`
	}

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

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var metas []Meta

	var dtos []JSONDTO
	if err := json.Unmarshal(data, &dtos); err != nil {
		return nil, err
	}

	for _, dto := range dtos {
		vcsRepoRoot, err := url.Parse(dto.RootRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to parse vcs repo root: %w", err)
		}

		imp := MetaImport{
			Prefix: dto.ImportPrefix,
			VCS: MetaImportVCS{
				Name:     dto.VCS,
				RepoRoot: vcsRepoRoot,
			},
		}

		src := MetaSource{
			HomepageURL:      dto.HomepageURL,
			DirectoryPattern: dto.DirectoryPattern,
			FilePattern:      dto.FilePattern,
		}

		if src.HomepageURL == "" {
			src.HomepageURL = imp.VCS.RepoRoot.String()
		}

		if strings.Contains(imp.VCS.RepoRoot.Host, "github.com") {
			if zerokit.IsZero(src.DirectoryPattern) {
				src.DirectoryPattern = fmt.Sprintf("%s/tree/master{/dir}", imp.VCS.RepoRoot.String())
			}
			if zerokit.IsZero(src.FilePattern) {
				src.FilePattern = fmt.Sprintf("%s/{file}#L{line}", src.DirectoryPattern)
			}
		}

		var nested []MetaNestedModule

		if 0 < len(dto.Submodules) {
			for _, modPath := range dto.Submodules {
				nested = append(nested, MetaNestedModule{
					Path: modPath,
				})
			}
		}

		metas = append(metas, Meta{
			Import: imp,
			Source: src,
			Nested: nested,
		})
	}

	return metas, nil
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
