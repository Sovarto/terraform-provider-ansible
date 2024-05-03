package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &PlaybookResource{}
var _ resource.ResourceWithImportState = &PlaybookResource{}

func NewPlaybookResource() resource.Resource {
	return &PlaybookResource{}
}

type PlaybookResource struct {
}

// PlaybookResourceModel describes the resource data model.
type PlaybookResourceModel struct {
	Playbook              types.String `tfsdk:"playbook"`
	Inventory             types.String `tfsdk:"inventory"`
	StoreOutputInState    types.Bool   `tfsdk:"store_output_in_state"`
	AnsiblePlaybookBinary types.String `tfsdk:"ansible_playbook_binary"`
	ExtraVars             types.Map    `tfsdk:"extra_vars"`
	ArtifactQueries       types.Map    `tfsdk:"artifact_queries"`
	PlaybookHash          types.String `tfsdk:"playbook_hash"`
	AnsiblePlaybookStdout types.String `tfsdk:"ansible_playbook_stdout"`
	AnsiblePlaybookStderr types.String `tfsdk:"ansible_playbook_stderr"`
	Id                    types.String `tfsdk:"id"`
}

type ArtifactQueryModel struct {
	JSONPath         types.String `tfsdk:"jsonpath"`
	Result           types.String `tfsdk:"result"`
	FailOnMissingKey types.Bool   `tfsdk:"fail_on_missing_key"`
	JsonOutput       types.Bool   `tfsdk:"json_output"`
}

func (ArtifactQueryModel) AttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"jsonpath":            types.StringType,
		"result":              types.StringType,
		"fail_on_missing_key": types.BoolType,
		"json_output":         types.BoolType,
	}
}

func (m ArtifactQueryModel) Value(ctx context.Context, query *ArtifactQuery) diag.Diagnostics {
	var diags diag.Diagnostics

	query.JSONPath = m.JSONPath.ValueString()
	query.Result = m.Result.ValueString()
	query.FailOnMissingKey = m.FailOnMissingKey.ValueBool()
	query.JsonOutput = m.JsonOutput.ValueBool()

	return diags
}

func (m *ArtifactQueryModel) Set(ctx context.Context, query ArtifactQuery) diag.Diagnostics {
	var diags diag.Diagnostics

	m.JSONPath = types.StringValue(query.JSONPath)
	m.Result = types.StringValue(query.Result)
	m.FailOnMissingKey = types.BoolValue(query.FailOnMissingKey)
	m.JsonOutput = types.BoolValue(query.JsonOutput)

	return diags
}

func (r *PlaybookResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_playbook"
}

func (r *PlaybookResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Provides an Ansible playbook resource.",

		Attributes: map[string]schema.Attribute{
			"playbook": schema.StringAttribute{
				MarkdownDescription: "Path to ansible playbook.",
				Optional:            false,
				Required:            true,
			},
			"inventory": schema.StringAttribute{
				MarkdownDescription: "The inventory to use. Not a path, the contents.",
				Optional:            false,
				Required:            true,
			},
			"store_output_in_state": schema.BoolAttribute{
				MarkdownDescription: "Whether or not to store the output of running Ansible in the state. Enable only for debugging, because this is usually huge and may contain sensitive data.",
				Optional:            true,
				Required:            false,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"ansible_playbook_binary": schema.StringAttribute{
				Required: false,
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("ansible-playbook"),
			},
			"extra_vars": schema.MapAttribute{
				Required:    false,
				Optional:    true,
				ElementType: types.StringType,
				Description: "A map of additional variables as: { keyString = \"value-1\", keyList = [\"list-value-1\", \"list-value-2\"], ... }.",
			},
			// From https://github.com/marshallford/terraform-provider-ansible/blob/2bbba6be0a59dd5b03e46e339a42032014662f67/internal/provider/navigator_run_resource.go#L429C1-L445C6
			"artifact_queries": schema.MapNestedAttribute{
				Description:         "Query the playbook artifact with JSONPath. The playbook artifact - the JSON output as generated by the JSON Callback Plugin - contains detailed information about every play and task from the playbook run.",
				MarkdownDescription: "Query the playbook artifact with [JSONPath](https://goessner.net/articles/JsonPath/). The playbook artifact - the JSON output as generated by the [JSON Callback Plugin](https://docs.ansible.com/ansible/2.9/plugins/callback/json.html) - contains detailed information about every play and task from the playbook run.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"jsonpath": schema.StringAttribute{
							Description: "JSONPath expression.",
							Required:    true,
						},
						"json_output": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Required:    false,
							Default:     booldefault.StaticBool(false),
							Description: "Output the result as valid JSON. Set this to true, if you select a whole sub-object or multiple values. Leave it at false, if you select the value of a single property.",
						},
						"fail_on_missing_key": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Required:    false,
							Default:     booldefault.StaticBool(false),
							Description: "Fail the resource, if there is no key specified by the JSON path",
						},
						"result": schema.StringAttribute{
							Description: "Result of the query. Result may be empty if a field or map key cannot be located.",
							Computed:    true,
						},
					},
				},
			},
			"playbook_hash": schema.StringAttribute{
				Computed:    true,
				Description: "Hash of playbook.",
			},
			"ansible_playbook_stdout": schema.StringAttribute{
				Computed:    true,
				Description: "An ansible-playbook CLI stdout output.",
			},
			"ansible_playbook_stderr": schema.StringAttribute{
				Computed:    true,
				Description: "An ansible-playbook CLI stderr output.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *PlaybookResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}
}

func (r *PlaybookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PlaybookResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	data.Id = types.StringValue(uuid.New().String())

	Execute(ctx, &resp.Diagnostics, &data)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PlaybookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}

func (r *PlaybookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PlaybookResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	Execute(ctx, &resp.Diagnostics, &data)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PlaybookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PlaybookResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *PlaybookResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {

	var plan *PlaybookResourceModel
	var config *PlaybookResourceModel
	var state *PlaybookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if plan == nil || config == nil {
		return
	}

	if !config.StoreOutputInState.ValueBool() {
		resp.Plan.SetAttribute(ctx, path.Root("ansible_playbook_stdout"), types.StringValue(""))
	}

	currentHash, err := calculatePlaybookHash(config.Playbook.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Calculating Playbook Hash", err.Error())
		return
	}

	planHash := types.StringValue(currentHash)
	resp.Plan.SetAttribute(ctx, path.Root("playbook_hash"), planHash)
	if state == nil || !plan.Playbook.Equal(state.Playbook) || !plan.Inventory.Equal(state.Inventory) ||
		!plan.ExtraVars.Equal(state.ExtraVars) || !planHash.Equal(state.PlaybookHash) {

		if config.StoreOutputInState.ValueBool() {
			resp.Plan.SetAttribute(ctx, path.Root("ansible_playbook_stdout"), types.StringUnknown())
		}
		resp.Plan.SetAttribute(ctx, path.Root("ansible_playbook_stderr"), types.StringUnknown())
		var queriesModel map[string]ArtifactQueryModel
		resp.Diagnostics.Append(plan.ArtifactQueries.ElementsAs(ctx, &queriesModel, false)...)

		for name, model := range queriesModel {
			model.Result = types.StringUnknown()
			queriesModel[name] = model
		}
		newQueriesModel, newDiags := types.MapValueFrom(ctx, types.ObjectType{AttrTypes: ArtifactQueryModel{}.AttrTypes()}, queriesModel)
		resp.Diagnostics.Append(newDiags...)
		resp.Plan.SetAttribute(ctx, path.Root("artifact_queries"), newQueriesModel)
	}
}

func (r *PlaybookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func calculatePlaybookHash(playbookPath string) (string, error) {
	roles, err := ParsePlaybookRoles(playbookPath)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't parse playbook roles! %s", err)
	}

	hash := sha256.New()
	for _, role := range roles {
		err := HashDirectory(hash, filepath.Join(filepath.Dir(playbookPath), "roles", role))
		if err != nil {
			return "", fmt.Errorf("ERROR: couldn't hash playbook roles! %s", err)
		}
	}

	err = HashFile(hash, playbookPath)
	if err != nil {
		return "", fmt.Errorf("ERROR: couldn't hash playbook! %s", err)
	}

	playbook_hash := hex.EncodeToString(hash.Sum(nil))
	return playbook_hash, nil
}
