package providerutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/util/jsonpath"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

/*
	CREATE OPTIONS
*/

func InterfaceToString(arr []interface{}) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	result := []string{}

	for _, val := range arr {
		tmpVal, ok := val.(string)
		if !ok {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Error: couldn't parse value to string!",
			})
		}

		result = append(result, tmpVal)
	}

	return result, diags
}

// Create a "verbpse" switch
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

func BuildInventory(inventoryDest string, inventoryContent string) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	// Check if inventory file is already present
	// if not, create one
	fileInfo, err := os.CreateTemp("", inventoryDest)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Failed to create inventory file: %v", err),
		})
	}

	tempFileName := fileInfo.Name()
	log.Printf("Inventory %s was created", fileInfo.Name())

	err = os.WriteFile(tempFileName, []byte(inventoryContent), 0o600)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Failed to create inventory: %v", err),
		})
	}

	return tempFileName, diags
}

func RemoveFile(filename string) diag.Diagnostics {
	var diags diag.Diagnostics

	err := os.Remove(filename)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Failed to remove file %s: %v", filename, err),
		})
	}

	return diags
}

func GetAllInventories(inventoryPrefix string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	tempDir := os.TempDir()

	log.Printf("[TEMP DIR]: %s", tempDir)

	files, err := os.ReadDir(tempDir)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Failed to read dir %s: %v", tempDir, err),
		})
	}

	inventories := []string{}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), inventoryPrefix) {
			inventoryAbsPath := filepath.Join(tempDir, file.Name())
			inventories = append(inventories, inventoryAbsPath)
		}
	}

	return inventories, diags
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
func jsonPath(data []byte, template string) (string, error) {
	var blob interface{}
	if err := json.Unmarshal(data, &blob); err != nil {
		return "", err
	}

	jsonPath := jsonpath.New(template)
	jsonPath.AllowMissingKeys(true)

	err := jsonPath.Parse(fmt.Sprintf("{%s}", template))
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
	JSONPath string
	Result   string
}

func QueryPlaybookArtifact(stdout bytes.Buffer, queries map[string]ArtifactQuery) error {

	for name, query := range queries {
		result, err := jsonPath(stdout.Bytes(), query.JSONPath)
		if err != nil {
			return fmt.Errorf("failed to query playbook artifact with JSONPath, %w", err)
		}

		query.Result = result
		queries[name] = query
	}

	return nil
}


