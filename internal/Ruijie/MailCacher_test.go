package ruijie

import (
	"strings"
	"testing"
)

func TestBodyPlainTextRemovesRuijieBoilerplate(t *testing.T) {
	body := graphBody{
		ContentType: "text",
		Content: `Dear customers,
Ruijie Cloud has detected that an alarm has eliminated at your network.
Please see below for the alarm detail:


Alarm source: Millinium_Hotel
Alarm type: Multiple DHCP servers on WAN port
Alarm time: 2026-04-08 09:20:57
Alarm clearance time: 2026-04-08 09:23:03
Alarm duration time: 0 day(s) 0 hour(s) 2 minute(s) 6 second(s)

Multiple DHCP server conflict on WAN port: MAC:00:ee:ab:08:de:89,IP:10.123.1.17,VLAN ID:105;
MAC:00:ee:ab:8f:ba:c9,IP:10.123.1.17,VLAN ID:105


Check here for more alarm details

***This is an automated e-mail. Please do not reply to this****

Best Regards,
Ruijie Cloud Team`,
	}

	got := bodyPlainText(body)
	want := `Alarm source: Millinium_Hotel
Alarm type: Multiple DHCP servers on WAN port
Alarm time: 2026-04-08 09:20:57
Alarm clearance time: 2026-04-08 09:23:03
Alarm duration time: 0 day(s) 0 hour(s) 2 minute(s) 6 second(s)
Multiple DHCP server conflict on WAN port: MAC:00:ee:ab:08:de:89,IP:10.123.1.17,VLAN ID:105;
MAC:00:ee:ab:8f:ba:c9,IP:10.123.1.17,VLAN ID:105`

	if got != want {
		t.Fatalf("bodyPlainText() mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}

	for _, fragment := range []string{
		"Dear customers",
		"Please see below",
		"Check here",
		"automated e-mail",
		"Best Regards",
		"Ruijie Cloud Team",
	} {
		if strings.Contains(got, fragment) {
			t.Fatalf("bodyPlainText() kept boilerplate fragment %q in:\n%s", fragment, got)
		}
	}
}

func TestRuijieAlarmFingerprintMatchesHappenedAndClearedForms(t *testing.T) {
	cleared := bodyPlainText(graphBody{
		ContentType: "text",
		Content: `Alarm source: Millinium_Hotel
Alarm type: Multiple DHCP servers on WAN port
Alarm time: 2026-04-08 10:20:56
Alarm clearance time: 2026-04-08 10:23:02
Alarm duration time: 0 day(s) 0 hour(s) 2 minute(s) 6 second(s)
Multiple DHCP server conflict on WAN port: MAC:00:ee:ab:08:de:89,IP:10.123.1.17,VLAN ID:105;`,
	})
	happened := bodyPlainText(graphBody{
		ContentType: "text",
		Content: `Ruijie Cloud has detected that an alarm has happened at your network.
Alarm source：Millinium_Hotel
Alarm level：Normal
Alarm type：Multiple DHCP servers on WAN port
Alarm time：2026-04-08 10:20:55
Multiple DHCP server conflict on WAN port: MAC:00:ee:ab:08:de:89,IP:10.123.1.17,VLAN ID:105;`,
	})

	sourceA, typeA, levelA := ruijieAlarmFields(cleared)
	sourceB, typeB, levelB := ruijieAlarmFields(happened)

	if sourceA != "Millinium_Hotel" || typeA != "Multiple DHCP servers on WAN port" || levelA != "" {
		t.Fatalf("unexpected cleared fields source=%q type=%q level=%q", sourceA, typeA, levelA)
	}
	if sourceB != "Millinium_Hotel" || typeB != "Multiple DHCP servers on WAN port" || levelB != "Normal" {
		t.Fatalf("unexpected happened fields source=%q type=%q level=%q", sourceB, typeB, levelB)
	}

	fpA := ruijieAlarmFingerprint(sourceA, typeA)
	fpB := ruijieAlarmFingerprint(sourceB, typeB)
	if fpA == "" || fpA != fpB {
		t.Fatalf("fingerprints should match, got %q and %q", fpA, fpB)
	}
	if strings.Contains(happened, "Ruijie Cloud has detected") {
		t.Fatalf("bodyPlainText() kept happened boilerplate in:\n%s", happened)
	}
}
