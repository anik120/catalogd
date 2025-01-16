package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

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
	jsonLineFormattedCompresableJSON, err := generateJSONLines([]byte(testCompressableJSON))
	require.NoError(t, err)
	yamlData, err := makeYAMLFromConcatenatedJSON([]byte(testCompressableJSON))
	require.NoError(t, err)
	jsonLineFormattedYamlData, err := generateJSONLines(yamlData)
	require.NoError(t, err)

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
		{
			name: "Ignores accept-encoding for the path /catalogs/test-catalog/api/v1/all with size < 1400 bytes",
			setupStore: func() error {
				return writeFile(filepath.Join(store.RootDir, "test-catalog2", "catalog.jsonl"), []byte(`{"foo":"bar"}`), 0600)
			},
			expectStatusOK:  true,
			expectedContent: `{"foo":"bar"}`,
			URLPath:         "/catalogs/test-catalog2/api/v1/all",
		},
		{
			name: "provides gzipped content for the path /catalogs/test-catalog/api/v1/all with size > 1400 bytes",
			setupStore: func() error {
				return writeFile(filepath.Join(store.RootDir, "test-catalog3", "catalog.jsonl"), []byte(testCompressableJSON), 0600)
			},
			expectStatusOK:  true,
			expectedContent: testCompressableJSON,
			URLPath:         "/catalogs/test-catalog3/api/v1/all",
		},
		{
			name: "Provides JSON-lines format for the served JSON catalog",
			setupStore: func() error {
				unpackResultFS := &fstest.MapFS{
					"catalog.json": &fstest.MapFile{Data: []byte(testCompressableJSON), Mode: os.ModePerm},
				}
				return store.Store(context.Background(), "test-catalog4", unpackResultFS)
			},
			expectStatusOK:  true,
			expectedContent: jsonLineFormattedCompresableJSON,
			URLPath:         "/catalogs/test-catalog4/api/v1/all",
		},
		{
			name:            "Provides JSON-lines format for the served YAML catalog",
			expectStatusOK:  true,
			expectedContent: jsonLineFormattedYamlData,
			URLPath:         fmt.Sprintf("%s/test-catalog/api/v1/all", urlPrefix),
			setupStore: func() error {
				yamlData, err := makeYAMLFromConcatenatedJSON([]byte(testCompressableJSON))
				if err != nil {
					return err
				}
				unpackResultFS := &fstest.MapFS{
					"catalog.yaml": &fstest.MapFile{Data: yamlData, Mode: os.ModePerm},
				}
				err = store.Store(context.Background(), "test-catalog", unpackResultFS)
				if err != nil {
					return err
				}
				return err
			},
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

// by default the compressor will only trigger for files larger than 1400 bytes
const testCompressableJSON = `{
  "defaultChannel": "stable-v6.x",
  "name": "cockroachdb",
  "schema": "olm.package"
}
{
  "entries": [
    {
      "name": "cockroachdb.v5.0.3"
    },
    {
      "name": "cockroachdb.v5.0.4",
      "replaces": "cockroachdb.v5.0.3"
    }
  ],
  "name": "stable-5.x",
  "package": "cockroachdb",
  "schema": "olm.channel"
}
{
  "entries": [
    {
      "name": "cockroachdb.v6.0.0",
      "skipRange": "<6.0.0"
    }
  ],
  "name": "stable-v6.x",
  "package": "cockroachdb",
  "schema": "olm.channel"
}
{
  "image": "quay.io/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8",
  "name": "cockroachdb.v5.0.3",
  "package": "cockroachdb",
  "properties": [
    {
      "type": "olm.gvk",
      "value": {
        "group": "charts.operatorhub.io",
        "kind": "Cockroachdb",
        "version": "v1alpha1"
      }
    },
    {
      "type": "olm.package",
      "value": {
        "packageName": "cockroachdb",
        "version": "5.0.3"
      }
    }
  ],
  "relatedImages": [
    {
      "name": "",
      "image": "quay.io/helmoperators/cockroachdb:v5.0.3"
    },
    {
      "name": "",
      "image": "quay.io/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8"
    }
  ],
  "schema": "olm.bundle"
}
{
  "image": "quay.io/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
  "name": "cockroachdb.v5.0.4",
  "package": "cockroachdb",
  "properties": [
    {
      "type": "olm.gvk",
      "value": {
        "group": "charts.operatorhub.io",
        "kind": "Cockroachdb",
        "version": "v1alpha1"
      }
    },
    {
      "type": "olm.package",
      "value": {
        "packageName": "cockroachdb",
        "version": "5.0.4"
      }
    }
  ],
  "relatedImages": [
    {
      "name": "",
      "image": "quay.io/helmoperators/cockroachdb:v5.0.4"
    },
    {
      "name": "",
      "image": "quay.io/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63"
    }
  ],
  "schema": "olm.bundle"
}
{
  "image": "quay.io/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba",
  "name": "cockroachdb.v6.0.0",
  "package": "cockroachdb",
  "properties": [
    {
      "type": "olm.gvk",
      "value": {
        "group": "charts.operatorhub.io",
        "kind": "Cockroachdb",
        "version": "v1alpha1"
      }
    },
    {
      "type": "olm.package",
      "value": {
        "packageName": "cockroachdb",
        "version": "6.0.0"
      }
    }
  ],
  "relatedImages": [
    {
      "name": "",
      "image": "quay.io/cockroachdb/cockroach-helm-operator:6.0.0"
    },
    {
      "name": "",
      "image": "quay.io/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba"
    }
  ],
  "schema": "olm.bundle"
}
`

// makeYAMLFromConcatenatedJSON takes a byte slice of concatenated JSON objects and returns a byte slice of concatenated YAML objects.
func makeYAMLFromConcatenatedJSON(data []byte) ([]byte, error) {
	var msg json.RawMessage
	var delimiter = []byte("---\n")
	var yamlData []byte

	yamlData = append(yamlData, delimiter...)

	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		err := dec.Decode(&msg)
		if errors.Is(err, io.EOF) {
			break
		}
		y, err := yaml.JSONToYAML(msg)
		if err != nil {
			return []byte{}, err
		}
		yamlData = append(yamlData, delimiter...)
		yamlData = append(yamlData, y...)
	}
	return yamlData, nil
}

// generateJSONLines takes a byte slice of concatenated JSON objects and returns a JSONlines-formatted string.
func generateJSONLines(in []byte) (string, error) {
	var out strings.Builder
	reader := bytes.NewReader(in)

	err := declcfg.WalkMetasReader(reader, func(meta *declcfg.Meta, err error) error {
		if err != nil {
			return err
		}

		if meta != nil && meta.Blob != nil {
			if meta.Blob[len(meta.Blob)-1] != '\n' {
				return fmt.Errorf("blob does not end with newline")
			}
		}

		_, err = out.Write(meta.Blob)
		if err != nil {
			return err
		}
		return nil
	})
	return out.String(), err
}
