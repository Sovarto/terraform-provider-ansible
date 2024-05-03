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

type Host struct {
	Failed      bool   `json:"failed"`
	Unreachable bool   `json:"unreachable"`
	Stderr      string `json:"stderr"`
	Stdout      string `json:"stdout"`
	Msg         string `json:"msg"`
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

						if len(host.Msg) > 0 {
							output += fmt.Sprintf("      Msg:\t%s\n", host.Msg)
						}
						if len(host.Stderr) > 0 {
							output += fmt.Sprintf("      Stderr:\t%s\n", host.Stderr)
						}
						if len(host.Stdout) > 0 {
							output += fmt.Sprintf("      Stdout:\t%s\n", host.Stdout)
						}
					}
				}
			}
		}
	}
	return output, failureDetected, nil
}
