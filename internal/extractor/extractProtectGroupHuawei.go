package extractor

import (
	"regexp"
	"strconv"
	"strings"
)

// HuaweiProtectMember represents one row in the protect-group member table.
type HuaweiProtectMember struct {
	Member     string `json:"member"`
	Role       string `json:"role"`      // work or protect
	Operation  string `json:"operation"` // none, auto switch src, auto switch dst
	State      string `json:"state"`     // active or standby
	PeerMember string `json:"peer_member"`
}

// HuaweiProtectGroup represents one protection group from "display protect-group".
type HuaweiProtectGroup struct {
	GroupID    int                   `json:"group_id"`
	AdminState string                `json:"admin_state"`
	Members    []HuaweiProtectMember `json:"members"`
}

var reGroupID = regexp.MustCompile(`(?m)Group ID\s*:\s*(\d+)`)
var reAdminState = regexp.MustCompile(`(?m)Admin State\s*:\s*(\S+)`)

// Member row: "  0/0/6         work         auto switch src   standby       0/3/6"
// Supports: none, auto switch src, auto switch dst for Operation
var reMemberRow = regexp.MustCompile(`(?m)^\s*(\d+/\d+/\d+)\s+(work|protect)\s+(none|auto\s+switch\s+(?:src|dst))\s+(active|standby)\s+(\S+)\s*$`)
// Fallback: allow optional spaces in slot (e.g. "0/ 0/ 6") and trailing junk (ANSI, etc.)
var reMemberRowFlex = regexp.MustCompile(`(?m)^\s*(\d+\s*/\s*\d+\s*/\s*\d+)\s+(work|protect)\s+(none|auto\s+switch\s+(?:src|dst))\s+(active|standby)\s+(\S+)`)

// stripANSICodes removes CSI escape sequences that may appear in SSH output.
func stripANSICodes(s string) string {
	return regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`).ReplaceAllString(s, "")
}

// ExtractHuaweiProtectGroups parses "display protect-group" output and returns all groups.
func ExtractHuaweiProtectGroups(output string) []HuaweiProtectGroup {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	output = stripANSICodes(output)

	groupIDs := reGroupID.FindAllStringSubmatch(output, -1)
	adminStates := reAdminState.FindAllStringSubmatch(output, -1)
	memberRows := reMemberRow.FindAllStringSubmatch(output, -1)
	if len(memberRows) == 0 {
		memberRows = reMemberRowFlex.FindAllStringSubmatch(output, -1)
		// Normalize slot format for flex matches: "0/ 0/ 6" -> "0/0/6"
		for i := range memberRows {
			memberRows[i][1] = strings.ReplaceAll(memberRows[i][1], " ", "")
		}
	}

	if len(memberRows) == 0 {
		return nil
	}

	// Build group list from member rows (each group has 2 members: work + protect)
	var groups []HuaweiProtectGroup
	var members []HuaweiProtectMember
	groupIdx := 0

	for _, m := range memberRows {
		mem := HuaweiProtectMember{
			Member:     m[1],
			Role:       m[2],
			Operation:  m[3],
			State:      m[4],
			PeerMember: m[5],
		}
		members = append(members, mem)

		// Each group has exactly 2 rows (work + protect)
		if len(members) == 2 {
			groupID := groupIdx + 1
			adminState := "enable"
			if groupIdx < len(groupIDs) {
				groupID, _ = strconv.Atoi(groupIDs[groupIdx][1])
			}
			if groupIdx < len(adminStates) {
				adminState = adminStates[groupIdx][1]
			}

			groups = append(groups, HuaweiProtectGroup{
				GroupID:    groupID,
				AdminState: adminState,
				Members:    append([]HuaweiProtectMember{}, members...),
			})
			members = members[:0]
			groupIdx++
		}
	}

	// Handle odd last group (single member) if any
	if len(members) == 1 {
		groupID := groupIdx + 1
		adminState := "enable"
		if groupIdx < len(groupIDs) {
			groupID, _ = strconv.Atoi(groupIDs[groupIdx][1])
		}
		if groupIdx < len(adminStates) {
			adminState = adminStates[groupIdx][1]
		}
		groups = append(groups, HuaweiProtectGroup{
			GroupID:    groupID,
			AdminState: adminState,
			Members:    members,
		})
	}

	return groups
}
