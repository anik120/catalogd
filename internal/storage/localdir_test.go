package storage

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

const urlPrefix = "/catalogs/"

var ctx = context.Background()

func TestLocalDirStorage(t *testing.T) {
	catalog := "test-catalog"
	testBundleName := "bundle.v0.0.1"
	testBundleImage := "quaydock.io/namespace/bundle:0.0.3"
	testBundleRelatedImageName := "test"
	testBundleRelatedImageImage := "testimage:latest"
	testBundleObjectData := "dW5pbXBvcnRhbnQK"
	testPackageDefaultChannel := "preview_test"
	testPackageName := "webhook_operator_test"
	testChannelName := "preview_test"

	testPackage := fmt.Sprintf(testPackageTemplate, testPackageDefaultChannel, testPackageName)
	testBundle := fmt.Sprintf(testBundleTemplate, testBundleImage, testBundleName, testPackageName, testBundleRelatedImageName, testBundleRelatedImageImage, testBundleObjectData)
	testChannel := fmt.Sprintf(testChannelTemplate, testPackageName, testChannelName, testBundleName)

	baseURL := &url.URL{Scheme: "http", Host: "test-addr", Path: urlPrefix}
	rootDir := t.TempDir()
	store := &LocalDirV1{RootDir: rootDir, RootURL: baseURL}
	unpackResult := &fstest.MapFS{
		"bundle.yaml":  {Data: []byte(testBundle), Mode: os.ModePerm},
		"package.yaml": {Data: []byte(testPackage), Mode: os.ModePerm},
		"channel.yaml": {Data: []byte(testChannel), Mode: os.ModePerm},
	}
	err := store.Store(ctx, catalog, unpackResult)
	require.NoError(t, err)

	// Verify stored content
	fbcFile := filepath.Join(rootDir, catalog, "catalog.jsonl")
	_, err = os.Stat(fbcFile)
	require.NoError(t, err)

	gotConfig, err := declcfg.LoadFS(ctx, unpackResult)
	require.NoError(t, err)
	storedConfig, err := declcfg.LoadFile(os.DirFS(filepath.Dir(fbcFile)), filepath.Base(fbcFile))
	require.NoError(t, err)

	require.Equal(t, cmp.Diff(gotConfig, storedConfig), "")

	// Verify content URL
	expectedURL := baseURL.JoinPath(catalog).String()
	require.Equal(t, expectedURL, store.BaseURL(catalog))

	require.True(t, store.ContentExists(catalog))

	// Delete stored content
	err = store.Delete(catalog)
	require.NoError(t, err)
	_, err = os.Stat(fbcFile)
	require.True(t, os.IsNotExist(err))
	require.False(t, store.ContentExists(catalog))
}

func TestLocalDirServerHandler(t *testing.T) {
	store := &LocalDirV1{RootDir: t.TempDir(), RootURL: &url.URL{Path: urlPrefix}}
	testServer := httptest.NewServer(store.StorageServerHandler())
	defer testServer.Close()

	for _, tc := range []struct {
		name            string
		setupStore      func() error
		expectStatusOK  bool
		expectedContent string
		URLPath         string
	}{
		{
			name:            "Server returns 404 when root URL is queried",
			setupStore:      func() error { return nil },
			expectStatusOK:  false,
			expectedContent: "",
			URLPath:         "",
		},
		{
			name:            "Server returns 404 when path '/' is queried",
			setupStore:      func() error { return nil },
			expectStatusOK:  false,
			expectedContent: "",
			URLPath:         "/",
		},
		{
			name:            "Server returns 404 when path '/catalogs/' is queried",
			setupStore:      func() error { return nil },
			expectStatusOK:  false,
			expectedContent: "",
			URLPath:         "/catalogs/",
		},
		{
			name:            "Server return 404 when path '/catalogs/test-catalog/' is queried",
			setupStore:      func() error { return nil },
			expectStatusOK:  false,
			expectedContent: "",
			URLPath:         "/catalogs/test-catalog/",
		},
		{
			name:            "Server return 404 when path '/catalogs/test-catalog/api/' is queried",
			setupStore:      func() error { return nil },
			expectStatusOK:  false,
			expectedContent: "",
			URLPath:         "/catalogs/test-catalog/api/",
		},
		{
			name:            "Serer return 404 when path '/catalogs/test-catalog/api/v1' is queried",
			setupStore:      func() error { return nil },
			expectStatusOK:  false,
			expectedContent: "",
			URLPath:         "/catalogs/test-catalog/api/v1c",
		},
		{
			name:            "Server return 404 when path '/catalogs/test-catalog/non-existent.txt' is queried",
			setupStore:      func() error { return nil },
			expectStatusOK:  false,
			expectedContent: "",
			URLPath:         "/catalogs/test-catalog/non-existent.txt",
		},
		{
			name: "Server returns 404 when path '/catalogs/test-catalog.jsonl' is queried even if the file exists, since we don't serve the filesystem, and serve an API instead",
			setupStore: func() error {
				return writeFile(filepath.Join(store.RootDir, "test-catalog", "catalog.jsonl"), []byte(`{"foo":"bar"}`), 0600)
			},
			expectStatusOK:  false,
			expectedContent: `{"foo":"bar"}`,
			URLPath:         "/catalogs/test-catalog.jsonl",
		},
		{
			name: "Server return 200 when path '/catalogs/test-catalog/api/v1/all' is queried, when the file exists",
			setupStore: func() error {
				return writeFile(filepath.Join(store.RootDir, "test-catalog", "catalog.jsonl"), []byte(`{"foo":"bar"}`), 0600)
			},
			expectStatusOK:  true,
			expectedContent: `{"foo":"bar"}`,
			URLPath:         "/catalogs/test-catalog/api/v1/all",
		},
	} {
		require.NoError(t, tc.setupStore())
		if tc.expectStatusOK {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", testServer.URL, tc.URLPath), nil)
			require.NoError(t, err)
			req.Header.Set("Accept-Encoding", "gzip")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var actualContent []byte
			switch resp.Header.Get("Content-Encoding") {
			case "gzip":
				require.Greater(t, len(tc.expectedContent), 1400,
					fmt.Sprintf("gzipped content should only be provided for content larger than 1400 bytes, but our expected content is only %d bytes", len(tc.expectedContent)))
				gz, err := gzip.NewReader(resp.Body)
				require.NoError(t, err)
				actualContent, err = io.ReadAll(gz)
				require.NoError(t, err)
			default:
				require.Less(t, len(tc.expectedContent), 1400,
					fmt.Sprintf("plaintext content should only be provided for content smaller than 1400 bytes, but we received plaintext for %d bytes\n expectedContent:\n%s\n", len(tc.expectedContent), []byte(tc.expectedContent)))
				actualContent, err = io.ReadAll(resp.Body)
				require.NoError(t, err)
			}

			require.Equal(t, []byte(tc.expectedContent), actualContent)
			require.NoError(t, resp.Body.Close())
		} else {
			resp, err := http.Get(fmt.Sprintf("%s/%s", testServer.URL, tc.URLPath)) //nolint:gosec
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusNotFound, resp.StatusCode)
		}
	}
}

func writeFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, mode)
}

const testBundleTemplate = `---
image: %s
name: %s
schema: olm.bundle
package: %s
relatedImages:
  - name: %s
    image: %s
properties:
  - type: olm.bundle.object
    value:
      data: %s
  - type: some.other
    value:
      data: arbitrary-info
`

const testPackageTemplate = `---
defaultChannel: %s
name: %s
schema: olm.package
`

const testChannelTemplate = `---
schema: olm.channel
package: %s
name: %s
entries:
  - name: %s
`
