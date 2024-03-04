package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/sovarto/terraform-provider-ansible/providerutils"
)

const ansiblePlaybook2 = "ansible-playbook2"

func resourcePlaybook2() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourcePlaybook2Create,
		ReadContext:   resourcePlaybook2Read,
		UpdateContext: resourcePlaybook2Update,
		DeleteContext: resourcePlaybook2Delete,

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

			"name": {
				Type:        schema.TypeString,
				Required:    false,
				Optional:    true,
				Description: "Name of the desired host on which the playbook will be executed.",
			},

			"replayable": {
				Type:     schema.TypeBool,
				Required: false,
				Optional: true,
				Default:  true,
				Description: "" +
					"If 'true', the playbook will be executed on every 'terraform apply' and with that, the resource" +
					" will be recreated. " +
					"If 'false', the playbook will be executed only on the first 'terraform apply'. " +
					"Note, that if set to 'true', when doing 'terraform destroy', it might not show in the destroy " +
					"output, even though the resource still gets destroyed.",
			},

			"ignore_playbook_failure": {
				Type:     schema.TypeBool,
				Required: false,
				Optional: true,
				Default:  false,
				Description: "This parameter is good for testing. " +
					"Set to 'true' if the desired playbook is meant to fail, " +
					"but still want the resource to run successfully.",
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

			"tags": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Required:    false,
				Optional:    true,
				Description: "List of tags of plays and tasks to run.",
			},

			"limit": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Required:    false,
				Optional:    true,
				Description: "List of hosts to exclude from the playbook execution.",
			},

			"check_mode": {
				Type:     schema.TypeBool,
				Required: false,
				Optional: true,
				Default:  false,
				Description: "If 'true', playbook execution won't make any changes but " +
					"only change predictions will be made.",
			},

			"diff_mode": {
				Type:     schema.TypeBool,
				Required: false,
				Optional: true,
				Default:  false,
				Description: "" +
					"If 'true', when changing (small) files and templates, differences in those files will be shown. " +
					"Recommended usage with 'check_mode'.",
			},

			"keep_temporary_inventory_file": {
				Type:        schema.TypeBool,
				Required:    false,
				Optional:    true,
				Default:     false,
				Description: "If 'true' will not delete the temporary inventory file. Use for troubleshooting outside of Terraform.",
			},

			// connection configs are handled with extra_vars
			"force_handlers": {
				Type:        schema.TypeBool,
				Required:    false,
				Optional:    true,
				Default:     false,
				Description: "If 'true', run handlers even if a task fails.",
			},

			"state_file": {
				Type:        schema.TypeString,
				Required:    false,
				Optional:    true,
				Description: "The path to the Terraform state file. See [the cloud.terraform documentation](https://github.com/ansible-collections/cloud.terraform/blob/main/docs/cloud.terraform.terraform_provider_inventory.rst#parameters) for more info.",
			},

			"project_path": {
				Type:        schema.TypeString,
				Required:    false,
				Optional:    true,
				Description: "The path to the Terraform project. When using CDKTF, set this to the folder of your stack inside cdktf.out/stacks, e.g. cdktf.out/stacks/mystack. See [the cloud.terraform documentation](https://github.com/ansible-collections/cloud.terraform/blob/main/docs/cloud.terraform.terraform_provider_inventory.rst#parameters) for more info",
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

			// Ansible Vault
			"vault_files": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Required:    false,
				Optional:    true,
				Description: "List of vault files.",
			},

			"vault_password_file": {
				Type:        schema.TypeString,
				Required:    false,
				Optional:    true,
				Default:     "",
				Description: "Path to a vault password file.",
			},

			"vault_id": {
				Type:        schema.TypeString,
				Required:    false,
				Optional:    true,
				Default:     "",
				Description: "ID of the desired vault(s).",
			},

			// computed
			// debug output
			"args": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Computed:    true,
				Description: "Used to build arguments to run Ansible playbook with.",
			},

			"temp_inventory_file": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Path to created temporary inventory file.",
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
func resourcePlaybook2Create(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	// required settings
	playbook, okay := data.Get("playbook").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'playbook'!",
			Detail:   ansiblePlaybook2,
		})
	}

	// optional settings
	name, okay := data.Get("name").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'name'!",
			Detail:   ansiblePlaybook2,
		})
	}

	verbosity, okay := data.Get("verbosity").(int)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'verbosity'!",
			Detail:   ansiblePlaybook2,
		})
	}

	tags, okay := data.Get("tags").([]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'tags'!",
			Detail:   ansiblePlaybook2,
		})
	}

	limit, okay := data.Get("limit").([]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'limit'!",
			Detail:   ansiblePlaybook2,
		})
	}

	checkMode, okay := data.Get("check_mode").(bool)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'check_mode'!",
			Detail:   ansiblePlaybook2,
		})
	}

	diffMode, okay := data.Get("diff_mode").(bool)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'diff_mode'!",
			Detail:   ansiblePlaybook2,
		})
	}

	forceHandlers, okay := data.Get("force_handlers").(bool)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'force_handlers'!",
			Detail:   ansiblePlaybook2,
		})
	}

	extraVars, okay := data.Get("extra_vars").(map[string]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'extra_vars'!",
			Detail:   ansiblePlaybook2,
		})
	}

	varFiles, okay := data.Get("var_files").([]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'var_files'!",
			Detail:   ansiblePlaybook2,
		})
	}

	vaultFiles, okay := data.Get("vault_files").([]interface{})
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'vault_files'!",
			Detail:   ansiblePlaybook2,
		})
	}

	vaultPasswordFile, okay := data.Get("vault_password_file").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'vault_password_file'!",
			Detail:   ansiblePlaybook2,
		})
	}

	vaultID, okay := data.Get("vault_id").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'vault_id'!",
			Detail:   ansiblePlaybook2,
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

	if name != "" {
		args = append(args, "-e", "hostname="+name)
	}

	if len(tags) > 0 {
		tmpTags := []string{}

		for _, tag := range tags {
			tagStr, okay := tag.(string)
			if !okay {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  "ERROR [%s]: couldn't assert type: string",
					Detail:   ansiblePlaybook2,
				})
			}

			tmpTags = append(tmpTags, tagStr)
		}

		tagsStr := strings.Join(tmpTags, ",")
		args = append(args, "--tags", tagsStr)
	}

	if len(limit) > 0 {
		tmpLimit := []string{}

		for _, l := range limit {
			limitStr, okay := l.(string)
			if !okay {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  "ERROR [%s]: couldn't assert type: string",
					Detail:   ansiblePlaybook2,
				})
			}

			tmpLimit = append(tmpLimit, limitStr)
		}

		limitStr := strings.Join(tmpLimit, ",")
		args = append(args, "--limit", limitStr)
	}

	if checkMode {
		args = append(args, "--check")
	}

	if diffMode {
		args = append(args, "--diff")
	}

	if len(varFiles) != 0 {
		for _, varFile := range varFiles {
			varFileString, okay := varFile.(string)
			if !okay {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  "ERROR [%s]: couldn't assert type: string",
					Detail:   ansiblePlaybook2,
				})
			}

			args = append(args, "-e", "@"+varFileString)
		}
	}

	// Ansible vault
	if len(vaultFiles) != 0 {
		for _, vaultFile := range vaultFiles {
			vaultFileString, okay := vaultFile.(string)
			if !okay {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  "ERROR [%s]: couldn't assert type: string",
					Detail:   ansiblePlaybook2,
				})
			}

			args = append(args, "-e", "@"+vaultFileString)
		}

		args = append(args, "--vault-id")

		vaultIDArg := ""
		if vaultID != "" {
			vaultIDArg += vaultID
		}

		if vaultPasswordFile != "" {
			vaultIDArg += "@" + vaultPasswordFile
		} else {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "ERROR [ansible-playbook]: can't access vault file(s)! Missing 'vault_password_file'!",
				Detail:   ansiblePlaybook2,
			})
		}

		args = append(args, vaultIDArg)
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
						Detail:   ansiblePlaybook2,
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
			Detail:   ansiblePlaybook2,
		})
	}

	if err := data.Set("temp_inventory_file", ""); err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("ERROR [ansible-playbook]: couldn't set 'temp_inventory_file'! %v", err),
			Detail:   ansiblePlaybook2,
		})
	}

	diagsFromUpdate := resourcePlaybook2Update(ctx, data, meta)
	diags = append(diags, diagsFromUpdate...)

	return diags
}

func resourcePlaybook2Read(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	replayable, okay := data.Get("replayable").(bool)

	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'replayable'!",
			Detail:   ansiblePlaybook2,
		})
	}

	// if (replayable == true) --> then we want to recreate (reapply) this resource: exits == false
	// if (replayable == false) --> we don't want to recreate (reapply) this resource: exists == true
	if replayable {
		// make sure to do destroy of this resource.
		resourcePlaybookDelete(ctx, data, meta)
	}

	return diags
}

func resourcePlaybook2Update(ctx context.Context, data *schema.ResourceData, _ interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ansiblePlaybookBinary, okay := data.Get("ansible_playbook_binary").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'ansible_playbook_binary'!",
			Detail:   ansiblePlaybook2,
		})
	}

	playbook, okay := data.Get("playbook").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'playbook'!",
			Detail:   ansiblePlaybook2,
		})
	}

	log.Printf("LOG [ansible-playbook]: playbook = %s", playbook)

	ignorePlaybookFailure, okay := data.Get("ignore_playbook_failure").(bool)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'ignore_playbook_failure'!",
			Detail:   ansiblePlaybook2,
		})
	}

	keepTemporaryInventoryFile, okay := data.Get("keep_temporary_inventory_file").(bool)

	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'keep_temporary_inventory_file'!",
			Detail:   ansiblePlaybook2,
		})
	}

	argsTf, okay := data.Get("args").([]interface{})

	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'args'!",
			Detail:   ansiblePlaybook2,
		})
	}

	tempInventoryFile, okay := data.Get("temp_inventory_file").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'temp_inventory_file'!",
			Detail:   ansiblePlaybook2,
		})
	}

	stateFile, okay := data.Get("state_file").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'state_file'!",
			Detail:   ansiblePlaybook2,
		})
	}

	projectPath, okay := data.Get("project_path").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'project_path'!",
			Detail:   ansiblePlaybook2,
		})
	}

	inventoryFileNamePrefix := ".inventory-"

	if tempInventoryFile == "" {
		tempFileName, diagsFromUtils := providerutils.BuildDynamicPlaybookInventory(
			inventoryFileNamePrefix+"*.yml",
			stateFile,
			projectPath,
		)
		tempInventoryFile = tempFileName

		diags = append(diags, diagsFromUtils...)

		if err := data.Set("temp_inventory_file", tempInventoryFile); err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "ERROR [ansible-playbook]: couldn't set 'temp_inventory_file'!",
				Detail:   ansiblePlaybook2,
			})
		}
	}

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
				Detail:   ansiblePlaybook2,
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
	err := runAnsiblePlay.Wait()
	wg.Wait() // Also wait for output processing to complete

	if err != nil {
		playbookFailMsg := stderrBuf.String()
		if !ignorePlaybookFailure {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  playbookFailMsg,
				Detail:   ansiblePlaybook2,
			})
		} else {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  playbookFailMsg,
				Detail:   ansiblePlaybook2,
			})
		}
	}

	if !keepTemporaryInventoryFile {
		diagsFromUtils := providerutils.RemoveFile(tempInventoryFile)

		diags = append(diags, diagsFromUtils...)

		if err := data.Set("temp_inventory_file", ""); err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "ERROR [ansible-playbook]: couldn't set 'temp_inventory_file'!",
				Detail:   ansiblePlaybook2,
			})
		}
	}

	// *******************************************************************************

	// NOTE: Calling `resourcePlaybook2Read` will make a call to `resourcePlaybook2Delete` which sets
	//		 data.SetId(""), so when replayable is true, the resource gets created and then immediately deleted.
	//		 This causes provider to fail, therefore we essentially can't call data.SetId("") during a create task

	// diagsFromRead := resourcePlaybookRead(ctx, data, meta)
	// diags = append(diags, diagsFromRead...)
	return diags
}

// On "terraform destroy", every resource removes its temporary inventory file.
func resourcePlaybook2Delete(_ context.Context, data *schema.ResourceData, _ interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	data.SetId("")

	tempInventoryFile, okay := data.Get("temp_inventory_file").(string)
	if !okay {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "ERROR [%s]: couldn't get 'temp_inventory_file'!",
			Detail:   ansiblePlaybook2,
		})
	}

	if tempInventoryFile != "" {
		providerutils.RemoveFile(tempInventoryFile)
	}

	return diags
}
