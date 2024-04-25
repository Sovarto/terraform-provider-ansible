package providerutils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

/*
	CREATE OPTIONS
*/

const DefaultHostGroup = "default"

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
