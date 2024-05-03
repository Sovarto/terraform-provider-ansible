package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/util/jsonpath"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Create a "verbose" switch
// example: verbosity = 2 --> verbose_switch = "-vv"
func CreateVerboseSwitch(verbosity int) string {
	verbose := ""

	if verbosity == 0 {
		return verbose
	}

	verbose += "-"
	verbose += strings.Repeat("v", verbosity)

	return verbose
}

func BuildInventory(ctx context.Context, inventoryDest string, inventoryContent string, diags *diag.Diagnostics) string {
	// Check if inventory file is already present
	// if not, create one
	fileInfo, err := os.CreateTemp("", inventoryDest)
	if err != nil {
		diags.AddError("Failed to create inventory file", err.Error())
	}

	tempFileName := fileInfo.Name()
	tflog.Debug(ctx, fmt.Sprintf("Inventory %s was created", fileInfo.Name()))

	err = os.WriteFile(tempFileName, []byte(inventoryContent), 0o600)
	if err != nil {
		diags.AddError("Failed to create inventory", err.Error())
	}

	return tempFileName
}

func RemoveFile(filename string, diags *diag.Diagnostics) {

	err := os.Remove(filename)
	if err != nil {
		diags.AddWarning(fmt.Sprintf("Failed to remove file %s", filename), err.Error())
	}
}

type AnsiblePlay struct {
	Roles []string `yaml:"roles"`
}

type AnsiblePlaybook []AnsiblePlay

func uniqueRoles(roles []string) []string {
	roleMap := make(map[string]bool)
	var unique []string
	for _, role := range roles {
		if _, exists := roleMap[role]; !exists {
			unique = append(unique, role)
			roleMap[role] = true
		}
	}
	return unique
}

func ParsePlaybookRoles(playbookPath string) ([]string, error) {
	var playbook AnsiblePlaybook
	content, err := os.ReadFile(playbookPath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(content, &playbook)
	if err != nil {
		return nil, err
	}

	// Extract roles from all plays
	var allRoles []string
	for _, play := range playbook {
		allRoles = append(allRoles, play.Roles...)
	}
	allRoles = uniqueRoles(allRoles)
	return allRoles, nil
}

func HashDirectory(hash hash.Hash, dirPath string) error {
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			err := HashFile(hash, path)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func HashFile(hash hash.Hash, filePath string) error {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	hash.Write(fileContent)
	return nil
}

// Adapted from https://github.com/marshallford/terraform-provider-ansible/blob/main/pkg/ansible/utils.go#L25
func jsonPath(data []byte, query ArtifactQuery) (string, error) {
	var blob interface{}
	if err := json.Unmarshal(data, &blob); err != nil {
		return "", err
	}

	jsonPath := jsonpath.New(query.JSONPath)
	jsonPath.AllowMissingKeys(!query.FailOnMissingKey)
	jsonPath.EnableJSONOutput(query.JsonOutput)

	err := jsonPath.Parse(fmt.Sprintf("{%s}", query.JSONPath))
	if err != nil {
		return "", err
	}

	output := new(bytes.Buffer)
	if err := jsonPath.Execute(output, blob); err != nil {
		return "", err
	}

	return output.String(), nil
}

// Adapted from https://github.com/marshallford/terraform-provider-ansible/blob/main/pkg/ansible/navigator_query.go#L9
type ArtifactQuery struct {
	JSONPath         string
	FailOnMissingKey bool
	JsonOutput       bool
	Result           string
}

func QueryPlaybookArtifact(stdout bytes.Buffer, queries map[string]ArtifactQuery) error {

	for name, query := range queries {
		result, err := jsonPath(stdout.Bytes(), query)
		if err != nil {
			return fmt.Errorf("failed to query playbook artifact with JSONPath, %w", err)
		}

		query.Result = result
		queries[name] = query
	}

	return nil
}
