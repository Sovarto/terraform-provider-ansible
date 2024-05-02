package ansible

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/sovarto/terraform-provider-ansible/providerutils"
)

const ansiblePlaybook = "ansible-playbook"

func resourcePlaybook() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourcePlaybookCreate,
		ReadContext:   resourcePlaybookRead,
		UpdateContext: resourcePlaybookUpdate,
		DeleteContext: resourcePlaybookDelete,
		CustomizeDiff: customizeDiff,

		Schema: map[string]*schema.Schema{
			// Required settings
			"playbook": {
				Type:        schema.TypeString,
				Required:    true,
				Optional:    false,
				Description: "Path to ansible playbook.",
			},

			// Optional settings
			"ansible_playbook_binary": {
				Type:        schema.TypeString,
				Required:    false,
				Optional:    true,
				Default:     "ansible-playbook",
				Description: "Path to ansible-playbook executable (binary).",
			},

			// ansible execution commands
			"verbosity": { // verbosity is between = (0, 6)
				Type:     schema.TypeInt,
				Required: false,
				Optional: true,
				Default:  0,
				Description: "A verbosity level between 0 and 6. " +
					"Set ansible 'verbose' parameter, which causes Ansible to print more debug messages. " +
					"The higher the 'verbosity', the more debug details will be printed.",
			},

			// connection configs are handled with extra_vars
			"force_handlers": {
				Type:        schema.TypeBool,
				Required:    false,
				Optional:    true,
				Default:     false,
				Description: "If 'true', run handlers even if a task fails.",
			},

			"inventory": {
				Type:        schema.TypeString,
				Required:    true,
				Optional:    false,
				Description: "The inventory to use.",
			},

			// become configs are handled with extra_vars --> these are also connection configs
			"extra_vars": {
				Type:        schema.TypeMap,
				Required:    false,
				Optional:    true,
				Description: "A map of additional variables as: { keyString = \"value-1\", keyList = [\"list-value-1\", \"list-value-2\"], ... }.",
			},

			"var_files": { // adds @ at the beginning of filename
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Required:    false,
				Optional:    true,
				Description: "List of variable files.",
			},

			"playbook_hash": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Hash of playbook.",
			},

			"ansible_playbook_stdout": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "An ansible-playbook CLI stdout output.",
			},

			"ansible_playbook_stderr": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "An ansible-playbook CLI stderr output.",
			},

			"artifact_queries": {
				Type:        schema.TypeMap,
				Description: "Query the playbook artifact with JSONPath. The playbook artifact contains detailed information about every play and task, as well as the stdout from the playbook run.",
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"jsonpath": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "JSONPath expression to query the artifact.",
						},
						"result": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Result of the query, serialized as a JSON string. Result may be empty if the specified field or map key cannot be located.",
						},
					},
				},
			},
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Minute), //nolint:gomnd
		},
	}
}

func customizeDiff(ctx context.Context, data *schema.ResourceDiff, meta interface{}) error {
	playbook, okay := data.Get("playbook").(string)
	if !okay {
		return fmt.Errorf("ERROR: couldn't get 'playbook'")
	}

	currentHash, err := CalculatePlaybookHash(playbook)
	if err != nil {
		return fmt.Errorf("error reading file content to hash: %s", err)
	}

	oldHash, okay := data.Get("playbook_hash").(string)
	if !okay || oldHash != currentHash {
		err = data.SetNew("playbook_hash", currentHash)
		if err != nil {
			return err
		}
	}

	return nil
}

//nolint:maintidx
func resourcePlaybookCreate(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	diagsFromUpdate := resourcePlaybookUpdate(ctx, data, meta)
	diags = append(diags, diagsFromUpdate...)

	return diags
}

func resourcePlaybookRead(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	return diags
}

func resourcePlaybookUpdate(ctx context.Context, data *schema.ResourceData, _ interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	allFieldsToRevert := []string{"playbook", "ansible_playbook_binary", "inventory", "verbosity", "force_handlers", "extra_vars", "var_files", "playbook_hash"}

	ansiblePlaybookBinary, okay := data.Get("ansible_playbook_binary").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't get 'ansible_playbook_binary'!",
			Detail:   ansiblePlaybook,
		})
	}

	playbook, okay := data.Get("playbook").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't get 'playbook'!",
			Detail:   ansiblePlaybook,
		})
	}

	inventory, okay := data.Get("inventory").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't get 'inventory'!",
			Detail:   ansiblePlaybook,
		})
	}

	log.Printf("LOG [ansible-playbook]: playbook = %s", playbook)

	verbosity, okay := data.Get("verbosity").(int)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't get 'verbosity'!",
			Detail:   ansiblePlaybook,
		})
	}

	forceHandlers, okay := data.Get("force_handlers").(bool)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't get 'force_handlers'!",
			Detail:   ansiblePlaybook,
		})
	}

	extraVars, okay := data.Get("extra_vars").(map[string]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't get 'extra_vars'!",
			Detail:   ansiblePlaybook,
		})
	}

	varFiles, okay := data.Get("var_files").([]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't get 'var_files'!",
			Detail:   ansiblePlaybook,
		})
	}

	artifactQueries, okay := data.Get("artifact_queries").(map[string]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't get 'artifact_queries'!",
			Detail:   ansiblePlaybook,
		})
	}

	/********************
	* 	PREP THE OPTIONS (ARGS)
	 */
	args := []string{}

	verbose := providerutils.CreateVerboseSwitch(verbosity)
	if verbose != "" {
		args = append(args, verbose)
	}

	if forceHandlers {
		args = append(args, "--force-handlers")
	}

	if len(varFiles) != 0 {
		for _, varFile := range varFiles {
			varFileString, okay := varFile.(string)
			if !okay {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  "ERROR [%s]: couldn't assert type: string",
					Detail:   ansiblePlaybook,
				})
			}

			args = append(args, "-e", "@"+varFileString)
		}
	}

	if len(extraVars) != 0 {
		for key, val := range extraVars {
			// Directly use the value if it's a string
			if strVal, ok := val.(string); ok {
				args = append(args, "-e", key+"="+strVal)
			} else {
				// For non-string values, create a JSON object with key-value pair
				jsonMap := map[string]interface{}{key: val}
				jsonBytes, err := json.Marshal(jsonMap)
				if err != nil {
					diags = append(diags, diag.Diagnostic{
						Severity: diag.Error,
						Summary:  fmt.Sprintf("ERROR [ansible-playbook]: couldn't convert value to JSON for key '%s'", key),
						Detail:   ansiblePlaybook,
					})
					continue
				}
				jsonStr := string(jsonBytes)
				args = append(args, "-e", jsonStr)
			}
		}
	}
	args = append(args, playbook)

	// set up the args
	log.Print("[ANSIBLE ARGS]:")
	log.Print(args)

	if diags.HasError() {
		RevertStateChanges(data, allFieldsToRevert...)
		return diags
	}

	inventoryFileNamePrefix := ".inventory-"

	tempFileName, diagsFromUtils := providerutils.BuildInventory(inventoryFileNamePrefix+"*.yml", inventory)
	tempInventoryFile := tempFileName

	diags = append(diags, diagsFromUtils...)

	log.Printf("Temp Inventory File: %s", tempInventoryFile)

	args = append(args, "-i", tempInventoryFile)

	if diags.HasError() {
		RevertStateChanges(data, allFieldsToRevert...)
		return diags
	}

	runAnsiblePlay := exec.Command(ansiblePlaybookBinary, args...)
	currentEnv := os.Environ()
	currentEnv = append(currentEnv, "ANSIBLE_STDOUT_CALLBACK=json")
	runAnsiblePlay.Env = currentEnv

	// Create pipes for the output and error streams
	stdoutPipe, err := runAnsiblePlay.StdoutPipe()
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Error creating stdout pipe",
			Detail:   err.Error(),
		})
		return diags
	}

	stderrPipe, err := runAnsiblePlay.StderrPipe()
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Error creating stderr pipe",
			Detail:   err.Error(),
		})
		return diags
	}

	// Start the command asynchronously
	if err := runAnsiblePlay.Start(); err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Error starting the command",
			Detail:   err.Error(),
		})
		return diags
	}

	var wg sync.WaitGroup
	wg.Add(2)

	var stderrBuf, stdoutBuf bytes.Buffer

	// Function to read and process output
	processOutput := func(pipe io.ReadCloser, buffer *bytes.Buffer, isStderr bool) {
		defer wg.Done()

		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			line := scanner.Text()
			buffer.WriteString(line + "\n")
			if isStderr {
				tflog.Error(ctx, line)
			} else {
				tflog.Info(ctx, line)
			}
		}

		if err := scanner.Err(); err != nil {
			var label string

			if isStderr {
				label = "STDERR"
			} else {
				label = "STDOUT"
			}

			diags = append(diags, diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  "Error reading output",
				Detail:   "There was an error reading " + label + ": " + err.Error(),
			})
		}
	}

	// Read from both outputs in separate goroutines
	go processOutput(stdoutPipe, &stdoutBuf, false)
	go processOutput(stderrPipe, &stderrBuf, true)

	// Wait for the command to finish
	err = runAnsiblePlay.Wait()
	wg.Wait() // Also wait for output processing to complete

	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Ansible playbook command finished with an error: " + err.Error(),
			Detail:   "STDERR:\n" + stderrBuf.String() + "\n\nSTDOUT:\n" + stdoutBuf.String(),
		})

		if diags.HasError() {
			RevertStateChanges(data, allFieldsToRevert...)
		}
	} else {
		if len(stderrBuf.String()) > 0 {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  "Error output from Ansible",
				Detail:   stderrBuf.String(),
			})
		}

		if len(artifactQueries) > 0 {
			queries := make(map[string]providerutils.ArtifactQuery)
			for key, rawValue := range artifactQueries {
				// Assert the value to the expected map type
				if queryMap, ok := rawValue.(map[string]interface{}); ok {
					// Initialize a new ArtifactQuery to hold the converted data
					var artifactQuery providerutils.ArtifactQuery

					// Safely attempt to retrieve and assign the JSONPath
					if jsonPath, exists := queryMap["jsonpath"].(string); exists {
						artifactQuery.JSONPath = jsonPath
					}

					// Assign the constructed ArtifactQuery to your result map
					queries[key] = artifactQuery
				}
			}

			err = providerutils.QueryPlaybookArtifact(stdoutBuf, queries)
			if err != nil {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  "Error querying playbook artifacts: " + err.Error(),
					Detail:   "STDERR:\n" + stderrBuf.String() + "\n\nSTDOUT:\n" + stdoutBuf.String(),
				})
			} else {
				for key, _ := range artifactQueries {
					data.Set(fmt.Sprintf("artifact_queries.%s.result", key), queries[key].Result)
				}
			}
		}

		if data.Id() == "" {
			data.SetId(time.Now().String())
		}

		if err := data.Set("ansible_playbook_stderr", stderrBuf.String()); err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "ERROR: couldn't set 'ansible_playbook_stderr' ",
				Detail:   err.Error(),
			})
		}

		if err := data.Set("ansible_playbook_stdout", stdoutBuf.String()); err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "ERROR: couldn't set 'ansible_playbook_stdout' ",
				Detail:   err.Error(),
			})
		}
	}

	diagsFromUtils = providerutils.RemoveFile(tempInventoryFile)

	diags = append(diags, diagsFromUtils...)

	return diags
}

func CalculatePlaybookHash(playbookPath string) (string, error) {
	roles, err := providerutils.ParsePlaybookRoles(playbookPath)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't parse playbook roles! %s", err)
	}

	hash := sha256.New()
	for _, role := range roles {
		err := providerutils.HashDirectory(hash, filepath.Join(filepath.Dir(playbookPath), "roles", role))
		if err != nil {
			return "", fmt.Errorf("ERROR: couldn't hash playbook roles! %s", err)
		}
	}

	err = providerutils.HashFile(hash, playbookPath)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't hash playbook! %s", err)
	}

	playbook_hash := hex.EncodeToString(hash.Sum(nil))
	return playbook_hash, nil
}

func resourcePlaybookDelete(_ context.Context, data *schema.ResourceData, _ interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	data.SetId("")

	return diags
}

func RevertStateChanges(data *schema.ResourceData, fields ...string) {
	for _, field := range fields {
		if data.HasChange(field) {
			previousValue, _ := data.GetChange(field)
			data.Set(field, previousValue)
		}
	}
}
