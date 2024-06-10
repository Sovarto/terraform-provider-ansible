package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Define structs to match the JSON structure
type HostStats struct {
	Failures    int `json:"failures"`
	Unreachable int `json:"unreachable"`
}

type Stats map[string]HostStats

type MsgType struct {
	StringValue string
	ArrayValue  []interface{}
	IsString    bool
}

func (m *MsgType) UnmarshalJSON(data []byte) error {
	if data[0] == '"' {
		m.IsString = true
		return json.Unmarshal(data, &m.StringValue)
	}
	m.IsString = false
	return json.Unmarshal(data, &m.ArrayValue)
}

type Result struct {
	Failed bool    `json:"failed"`
	Stderr string  `json:"stderr"`
	Stdout string  `json:"stdout"`
	Msg    MsgType `json:"msg"`
	Reason string  `json:"reason"`
}

type Host struct {
	Result
	Unreachable bool     `json:"unreachable"`
	Results     []Result `json:"results"`
}

type Task struct {
	Hosts map[string]Host `json:"hosts"`
	Task  struct {
		Name string `json:"name"`
	} `json:"task"`
}

type Play struct {
	Play struct {
		Name string `json:"name"`
	} `json:"play"`
	Tasks []Task `json:"tasks"`
}

type Root struct {
	Plays []Play `json:"plays"`
	Stats Stats  `json:"stats"`
}

func printFailedInfo(result Result, indent string) string {
	output := ""

	if result.Msg.IsString && len(result.Msg.StringValue) > 0 {
		output += fmt.Sprintf("%sMsg:\t%s\n", indent, result.Msg.StringValue)
	}
	if len(result.Reason) > 0 {
		output += fmt.Sprintf("%sReason:\t%s\n", indent, result.Reason)
	}
	if len(result.Stderr) > 0 {
		output += fmt.Sprintf("%sStderr:\t%s\n", indent, result.Stderr)
	}
	if len(result.Stdout) > 0 {
		output += fmt.Sprintf("%sStdout:\t%s\n", indent, result.Stdout)
	}

	return output
}

func AnalyzeJSON(buffer bytes.Buffer) (string, bool, error) {
	var root Root
	if err := json.Unmarshal(buffer.Bytes(), &root); err != nil {
		return "", false, err
	}

	// Check for failures or unreachable hosts
	failureDetected := false
	output := ""
	for _, stat := range root.Stats {
		if stat.Failures > 0 || stat.Unreachable > 0 {
			failureDetected = true
			break
		}
	}

	if failureDetected {
		for _, play := range root.Plays {
			playHeaderPrinted := false
			for _, task := range play.Tasks {
				taskHeaderPrinted := false
				for hostName, host := range task.Hosts {
					if host.Failed || host.Unreachable {
						if !playHeaderPrinted {
							output += fmt.Sprintf("PLAY <%s>\n", play.Play.Name)
							playHeaderPrinted = true
						}
						if !taskHeaderPrinted {
							output += fmt.Sprintf("  TASK <%s>\n", task.Task.Name)
							taskHeaderPrinted = true
						}

						output += fmt.Sprintf("    HOST <%s>\n", hostName)

						output += printFailedInfo(host.Result, "      ")
						if host.Results != nil && len(host.Results) > 0 {
							resultsOutput := ""
							for _, result := range host.Results {
								if result.Failed {
									resultsOutput += printFailedInfo(result, "        ")
								}
							}

							if len(resultsOutput) > 0 {
								output += "      RESULTS\n"
								output += resultsOutput
							}
						}
					}
				}
			}
		}
	}
	return output, failureDetected, nil
}
