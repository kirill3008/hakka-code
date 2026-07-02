package hakkacode

import (
	"fmt"
	"strings"

	"hakka-code/internal/hakkacode/transcript"
)

// renderedResult carries both the display text and clickable regions
// for a slash-command output that gets written into the transcript.
type renderedResult struct {
	text    string
	regions []transcript.ClickRegion
}

// shiftRegions offsets all ClickRegion Line values by n.
func shiftRegions(regions []transcript.ClickRegion, n int) []transcript.ClickRegion {
	if len(regions) == 0 {
		return nil
	}
	out := make([]transcript.ClickRegion, len(regions))
	for i, r := range regions {
		out[i] = r
		out[i].Line += n
	}
	return out
}

// renderDataInteractive works like renderData but also produces
// ClickRegions for interactive list commands.
func renderDataInteractive(cmd string, data map[string]any) (string, []transcript.ClickRegion) {
	switch cmd {
	case "session_list":
		return formatSessionListInteractive(data)
	case "model_list":
		return formatModelListInteractive(data)
	case "tool_list":
		return formatToolListInteractive(data)
	default:
		return renderData(cmd, data), nil
	}
}

// renderCommandResultInteractive renders the response to a slash command
// with clickable regions for interactive list results.
func renderCommandResultInteractive(cmd string, frame ResponseFrame) renderedResult {
	if frame.Error != "" {
		return renderedResult{text: fmt.Sprintf("error: %s\n", frame.Error)}
	}

	var body string
	var regions []transcript.ClickRegion
	switch frame.Type {
	case "session":
		body = renderSessionFrame(frame)
	case "result":
		body, regions = renderDataInteractive(cmd, frame.Data)
	case "done":
		if frame.Text != "" {
			body = frame.Text + "\n"
		} else {
			body = fmt.Sprintf("%s: ok\n", cmd)
		}
	default:
		body = fmt.Sprintf("%s: ok\n", cmd)
	}

	if header, ok := resultHeaders[cmd]; ok {
		return renderedResult{
			text:    fmt.Sprintf("\033[1m%s\033[0m\n%s", header, body),
			regions: shiftRegions(regions, 1),
		}
	}
	return renderedResult{text: body, regions: regions}
}

// formatSessionListInteractive renders the session list with clickable
// rows for switching sessions.
func formatSessionListInteractive(data map[string]any) (string, []transcript.ClickRegion) {
	sessions, _ := data["sessions"].([]any)
	if len(sessions) == 0 {
		return "no sessions\n", nil
	}

	var sb strings.Builder
	var regions []transcript.ClickRegion
	rowIdx := 0

	for _, item := range sessions {
		s, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := strField(s, "id")
		name := strField(s, "name")
		if name == "" {
			name = "(unnamed)"
		}
		mark := " "
		if boolField(s, "current") {
			mark = "*"
		}
		status := ""
		if boolField(s, "in_flight") {
			status = "[running]"
		}
		updated := localTime(strField(s, "updated_at"))
		msgs := fmt.Sprintf("%d", int(numField(s, "message_count")))

		row := fmt.Sprintf("%-1s %-36s %-30s %-3s %-19s %s",
			mark, id, name, msgs, updated, status)
		sb.WriteString(strings.TrimRight(row, " "))
		sb.WriteByte('\n')

		regions = append(regions, transcript.ClickRegion{
			Line:   rowIdx,
			Col:    2,
			Width:  36,
			Action: transcript.ClickAction{Action: "session-switch", Payload: id},
		})

		rowIdx++
	}
	return sb.String(), regions
}

func formatModelListInteractive(data map[string]any) (string, []transcript.ClickRegion) {
	models, _ := data["models"].([]any)
	if len(models) == 0 {
		return "no models\n", nil
	}

	var sb strings.Builder
	var regions []transcript.ClickRegion
	rowIdx := 0

	for _, item := range models {
		mm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		mark := " "
		if boolField(mm, "current") {
			mark = "*"
		}
		name := strField(mm, "name")
		sb.WriteString(fmt.Sprintf("%s %s\n", mark, name))

		regions = append(regions, transcript.ClickRegion{
			Line:   rowIdx,
			Col:    2,
			Width:  len(name),
			Action: transcript.ClickAction{Action: "model-switch", Payload: name},
		})
		rowIdx++
	}
	return sb.String(), regions
}

func formatToolListInteractive(data map[string]any) (string, []transcript.ClickRegion) {
	tools, _ := data["tools"].([]any)
	if len(tools) == 0 {
		return "no tools\n", nil
	}

	var sb strings.Builder
	var regions []transcript.ClickRegion
	rowIdx := 0

	for _, item := range tools {
		tm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strField(tm, "name")
		desc := sanitizeSnippet(strField(tm, "description"))
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		enabled := boolField(tm, "enabled")
		check := "[ ]"
		action := "tool-allow"
		if enabled {
			check = "[x]"
			action = "tool-deny"
		}
		row := fmt.Sprintf("%s %-30s %s", check, name, desc)
		sb.WriteString(strings.TrimRight(row, " "))
		sb.WriteByte('\n')

		regions = append(regions, transcript.ClickRegion{
			Line:   rowIdx,
			Col:    0,
			Width:  33,
			Action: transcript.ClickAction{Action: action, Payload: name},
		})
		rowIdx++
	}
	return sb.String(), regions
}
