package results_parser

import (
	"encoding/json"
	"fmt"
	"bytes"
)

// Define structs to match the JSON structure
type HostStats struct {
    Failures    int `json:"failures"`
    Unreachable int `json:"unreachable"`
}

type Stats map[string]HostStats

type Host struct {
    Failed bool   `json:"failed"`
    Stderr string `json:"stderr"`
}

type Task struct {
    Hosts map[string]Host `json:"hosts"`
    Task  struct {
        Name string `json:"name"`
    } `json:"task"`
}

type Play struct {
    Play  struct {
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
            for _, task := range play.Tasks {
                for hostName, host := range task.Hosts {
                    if host.Failed {
                        output += fmt.Sprintf("PLAY <%s>\n", play.Play.Name)
                        output += fmt.Sprintf("TASK <%s>\n", task.Task.Name)
                        output += fmt.Sprintf("%s:\n%s\n", hostName, host.Stderr)
                    }
                }
            }
        }
    }
    return output, failureDetected, nil
}