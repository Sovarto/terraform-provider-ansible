package provider

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func Execute(ctx context.Context, diags *diag.Diagnostics, data *PlaybookResourceModel) {

	var queriesModel map[string]ArtifactQueryModel
	diags.Append(data.ArtifactQueries.ElementsAs(ctx, &queriesModel, false)...)

	artifactQueries := map[string]ArtifactQuery{}
	for name, model := range queriesModel {
		var query ArtifactQuery

		diags.Append(model.Value(ctx, &query)...)
		artifactQueries[name] = query
	}

	args := []string{}

	var extraVars map[string]string
	diags.Append(data.ExtraVars.ElementsAs(ctx, &extraVars, false)...)

	if diags.HasError() {
		return
	}

	if len(extraVars) != 0 {
		for key, val := range extraVars {
			args = append(args, "-e", key+"="+val)
		}
	}

	args = append(args, data.Playbook.ValueString())
	tempInventoryFile := BuildInventory(ctx, ".inventory-*.yml", data.Inventory.ValueString(), diags)

	if diags.HasError() {
		return
	}

	args = append(args, "-i", tempInventoryFile)

	runAnsiblePlay := exec.Command(data.AnsiblePlaybookBinary.ValueString(), args...)
	currentEnv := os.Environ()
	currentEnv = append(currentEnv, "ANSIBLE_STDOUT_CALLBACK=json")
	runAnsiblePlay.Env = currentEnv

	var stdoutBuf, stderrBuf bytes.Buffer
	runAnsiblePlay.Stdout = &stdoutBuf
	runAnsiblePlay.Stderr = &stderrBuf

	executionError := runAnsiblePlay.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if len(stderr) > 0 {
		diags.AddWarning("Stderr from Ansible", stderr)
	}

	if executionError != nil {
		summary := "Ansible playbook command finished with an error: " + executionError.Error()
		details := ""

		formattedOutput, hadFailure, err := AnalyzeJSON(stdoutBuf)
		if err != nil {
			diags.AddError("Error analyzing result JSON: "+err.Error(), "STDOUT:\n"+stdout)
		} else if hadFailure {
			details = formattedOutput
		}

		diags.AddError(summary, details)
	} else {
		if data.StoreOutputInState.ValueBool() {
			data.AnsiblePlaybookStdout = types.StringValue(stdout)
		} else {
			data.AnsiblePlaybookStdout = types.StringValue("")
		}

		data.AnsiblePlaybookStderr = types.StringValue(stderr)

		err := QueryPlaybookArtifact(stdoutBuf, artifactQueries)
		if err != nil {
			diags.AddAttributeError(path.Root("artifact_queries"), "Playbook artifact queries failed", err.Error())
		}

		for name, model := range queriesModel {
			diags.Append(model.Set(ctx, artifactQueries[name])...)
			queriesModel[name] = model
		}

		newQueriesModel, newDiags := types.MapValueFrom(ctx, types.ObjectType{AttrTypes: ArtifactQueryModel{}.AttrTypes()}, queriesModel)
		diags.Append(newDiags...)
		data.ArtifactQueries = newQueriesModel

		formattedOutput, hadFailure, err := AnalyzeJSON(stdoutBuf)
		if err != nil {
			diags.AddError("Error analyzing result JSON: "+err.Error(), "STDERR:\n"+stderr+"\n\nSTDOUT:\n"+stdout)
		} else {
			if hadFailure {
				diags.AddWarning("Ansible results", formattedOutput)
			}
		}
	}

	RemoveFile(tempInventoryFile, diags)
}
