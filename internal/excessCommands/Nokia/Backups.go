package excesscommands

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/shell"
)

func Backups(username, password string) {
	cmd := "info configure flat"

	for olt := range shell.SendCommandNokiaOLTs(username, password, cmd) {
		if olt.Err != nil {
			log.Printf("ERROR %s: %v", olt.Host, olt.Err)
			continue
		}
		site := strings.ReplaceAll(olt.Site, "/", "-")
		if site == "" {
			site = "unknown"
		}
		folder := filepath.Join("backups", site, time.Now().Format("2006-01-02"))
		if err := os.MkdirAll(folder, 0o755); err != nil {
			log.Printf("ERROR mkdir %s: %v", folder, err)
			continue
		}

		name := strings.ReplaceAll(olt.Device, "/", "-")
		filename := fmt.Sprintf("%s_%s.txt", name, olt.Host)
		path := filepath.Join(folder, filename)

		if err := os.WriteFile(path, []byte(olt.Data), 0o644); err != nil {
			log.Printf("ERROR writing %s: %v", path, err)
			continue
		}
		fmt.Printf("saved: %s\n", path)
	}

}
