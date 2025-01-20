package serverutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"sigs.k8s.io/yaml"
)

// {
// 	name: "provides gzipped content for the path /catalogs/test-catalog/api/v1/all with size > 1400 bytes",
// 	setupStore: func() error {
// 		return writeFile(filepath.Join(store.RootDir, "test-catalog3", "catalog.jsonl"), []byte(testCompressableJSON), 0600)
// 	},
// 	expectStatusOK:  true,
// 	expectedContent: testCompressableJSON,
// 	URLPath:         "/catalogs/test-catalog3/api/v1/all",
// },

// jsonLineFormattedCompresableJSON, err := generateJSONLines([]byte(testCompressableJSON))
// 	require.NoError(t, err)
// {
// 	name: "Provides JSON-lines format for the served JSON catalog",
// 	setupStore: func() error {
// 		unpackResultFS := &fstest.MapFS{
// 			"catalog.json": &fstest.MapFile{Data: []byte(testCompressableJSON), Mode: os.ModePerm},
// 		}
// 		return store.Store(context.Background(), "test-catalog4", unpackResultFS)
// 	},
// 	expectStatusOK:  true,
// 	expectedContent: jsonLineFormattedCompresableJSON,
// 	URLPath:         "/catalogs/test-catalog4/api/v1/all",
// },

// yamlData, err := makeYAMLFromConcatenatedJSON([]byte(testCompressableJSON))
// require.NoError(t, err)
// jsonLineFormattedYamlData, err := generateJSONLines(yamlData)
// require.NoError(t, err)
// {
// 	name:            "Provides JSON-lines format for the served YAML catalog",
// 	expectStatusOK:  true,
// 	expectedContent: jsonLineFormattedYamlData,
// 	URLPath:         fmt.Sprintf("%s/test-catalog/api/v1/all", urlPrefix),
// 	setupStore: func() error {
// 		yamlData, err := makeYAMLFromConcatenatedJSON([]byte(testCompressableJSON))
// 		if err != nil {
// 			return err
// 		}
// 		unpackResultFS := &fstest.MapFS{
// 			"catalog.yaml": &fstest.MapFile{Data: yamlData, Mode: os.ModePerm},
// 		}
// 		err = store.Store(context.Background(), "test-catalog", unpackResultFS)
// 		if err != nil {
// 			return err
// 		}
// 		return err
// 	},
// },

// {
// 	name: "Ignores accept-encoding for the path /catalogs/test-catalog/api/v1/all with size < 1400 bytes",
// 	setupStore: func() error {
// 		return writeFile(filepath.Join(store.RootDir, "test-catalog2", "catalog.jsonl"), []byte(`{"foo":"bar"}`), 0600)
// 	},
// 	expectStatusOK:  true,
// 	expectedContent: `{"foo":"bar"}`,
// 	URLPath:         "/catalogs/test-catalog2/api/v1/all",
// },

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
