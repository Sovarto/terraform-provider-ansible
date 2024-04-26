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

			// computed
			// debug output
			"args": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Computed:    true,
				Description: "Used to build arguments to run Ansible playbook with.",
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
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Minute), //nolint:gomnd
		},
	}
}

//nolint:maintidx
func resourcePlaybookCreate(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	// required settings
	playbook, okay := data.Get("playbook").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'playbook'!",
			Detail:   ansiblePlaybook,
		})
	}

	verbosity, okay := data.Get("verbosity").(int)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'verbosity'!",
			Detail:   ansiblePlaybook,
		})
	}

	forceHandlers, okay := data.Get("force_handlers").(bool)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'force_handlers'!",
			Detail:   ansiblePlaybook,
		})
	}

	extraVars, okay := data.Get("extra_vars").(map[string]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'extra_vars'!",
			Detail:   ansiblePlaybook,
		})
	}

	varFiles, okay := data.Get("var_files").([]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'var_files'!",
			Detail:   ansiblePlaybook,
		})
	}

	// Generate ID
	data.SetId(time.Now().String())

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

	if err := data.Set("args", args); err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("ERROR [ansible-playbook]: couldn't set 'args'! %v", err),
			Detail:   ansiblePlaybook,
		})
	}

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

	if !data.HasChanges("ansible_playbook_binary", "playbook", "inventory") {
		return diags
	}

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

	roles, err := providerutils.ParsePlaybookRoles(playbook)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't parse playbook roles!",
			Detail:   err.Error(),
		})
	}

	hash := sha256.New()
	for _, role := range roles {
		err := providerutils.HashDirectory(hash, filepath.Join(filepath.Dir(playbook), "roles", role))
		if err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "ERROR: couldn't hash playbook!",
				Detail:   err.Error(),
			})
		}
	}

	playbook_hash := hex.EncodeToString(hash.Sum(nil))

	if playbook_hash == data.Get("playbook_hash").(string) {
		return diags
	}

	data.Set("playbook_hash", playbook_hash)

	argsTf, okay := data.Get("args").([]interface{})

	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR: couldn't get 'args'!",
			Detail:   ansiblePlaybook,
		})
	}

	inventoryFileNamePrefix := ".inventory-"

	tempFileName, diagsFromUtils := providerutils.BuildInventory(inventoryFileNamePrefix+"*.yml", inventory)
	tempInventoryFile := tempFileName

	diags = append(diags, diagsFromUtils...)

	log.Printf("Temp Inventory File: %s", tempInventoryFile)

	// ********************************* RUN PLAYBOOK ********************************

	args := []string{}

	args = append(args, "-i", tempInventoryFile)

	for _, arg := range argsTf {
		tmpArg, okay := arg.(string)
		if !okay {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "ERROR [ansible-playbook]: couldn't assert type: string",
				Detail:   ansiblePlaybook,
			})
		}

		args = append(args, tmpArg)
	}

	runAnsiblePlay := exec.Command(ansiblePlaybookBinary, args...)

	// Create pipes for the output and error streams
	stdoutPipe, _ := runAnsiblePlay.StdoutPipe()
	// if err != nil {
	// TODO: handle error
	// }

	stderrPipe, _ := runAnsiblePlay.StderrPipe()
	// if err != nil {
	// TODO: handle error
	// }

	// Start the command asynchronously
	runAnsiblePlay.Start()
	// if err := runAnsiblePlay.Start(); err != nil {
	// TODO: handle error
	// }

	// Use a wait group to wait for the output processing to complete
	var wg sync.WaitGroup
	wg.Add(2)

	var stderrBuf bytes.Buffer

	// Function to read and process output
	processOutput := func(pipe io.ReadCloser, isStderr bool) {
		defer wg.Done()

		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			line := scanner.Text()
			if isStderr {
				tflog.Error(ctx, line)
				stderrBuf.WriteString(line + "\n")
			} else {
				tflog.Debug(ctx, line)
			}
		}

		// if err := scanner.Err(); err != nil {
		// TODO: handle error
		// }
	}

	// Read from both outputs in separate goroutines
	go processOutput(stdoutPipe, false)
	go processOutput(stderrPipe, true)

	// Wait for the command to finish
	err = runAnsiblePlay.Wait()
	wg.Wait() // Also wait for output processing to complete

	if err != nil {
		playbookFailMsg := stderrBuf.String()
		if playbookFailMsg == "" {
			tflog.Error(ctx, "playbookFailMsg is empty although it shouldn't")
			playbookFailMsg = "playbookFailMsg is empty although it shouldn't"
		}

		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  playbookFailMsg,
			Detail:   ansiblePlaybook,
		})
	}

	diagsFromUtils = providerutils.RemoveFile(tempInventoryFile)

	diags = append(diags, diagsFromUtils...)

	return diags
}

func resourcePlaybookDelete(_ context.Context, data *schema.ResourceData, _ interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	data.SetId("")

	return diags
}
