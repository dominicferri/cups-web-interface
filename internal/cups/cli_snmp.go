package cups

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const snmpWalkPath = "/usr/bin/snmpwalk" // Net-SNMP, shipped with macOS

// snmpWalkBinary returns the snmpwalk to run, or "" if there is none.
func snmpWalkBinary() string {
	if _, err := os.Stat(snmpWalkPath); err == nil {
		return snmpWalkPath
	}
	p, _ := exec.LookPath("snmpwalk")
	return p
}

// cappedBuffer keeps the first max bytes and drops the rest, so a device that
// floods answers can't make the server hold an unbounded amount of output.
type cappedBuffer struct {
	bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

func (s *cliAccessorySource) SNMPAvailable() bool { return snmpWalkBinary() != "" }

// SNMPWalk runs one SNMPv2c snmpwalk per root. The community goes in a
// private snmp.conf, not on the command line, so other accounts can't read it
// from the process list.
func (s *cliAccessorySource) SNMPWalk(host, community string, roots []string) (string, error) {
	if !IsValidSNMPHost(host) || !IsValidSNMPCommunity(community) {
		return "", errors.New("invalid host or community")
	}
	bin := snmpWalkBinary()
	if bin == "" {
		return "", errors.New("snmpwalk not found")
	}
	dir, err := os.MkdirTemp("", "cups-webui-snmp-") // mode 0700
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	conf := "defVersion 2c\ndefCommunity " + community + "\n"
	if err := os.WriteFile(filepath.Join(dir, "snmp.conf"), []byte(conf), 0600); err != nil {
		return "", err
	}

	target := host
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		target = "udp6:[" + host + "]"
	}
	env := append(os.Environ(), "SNMPCONFPATH="+dir, "SNMP_PERSISTENT_DIR="+dir)

	// Under the server's 15-second write timeout, so a slow printer still gets
	// a response (with what it answered so far) instead of a dropped connection.
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	out := &cappedBuffer{max: 512 << 10}
	answered := false
	for _, root := range roots {
		var stdout cappedBuffer
		stdout.max = 512 << 10
		stderr := &cappedBuffer{max: 4 << 10}
		// -On numeric OIDs; -t/-r bound each request to about 4 seconds.
		cmd := exec.CommandContext(ctx, bin, "-On", "-t", "2", "-r", "1", target, root)
		cmd.Env = env
		cmd.Stdout = &stdout
		cmd.Stderr = stderr
		runErr := cmd.Run()
		got := len(ParseSNMPWalk(stdout.String())) > 0
		if got {
			answered = true
			out.WriteString(stdout.String())
			out.WriteString("\n")
		}
		if ctx.Err() != nil {
			break
		}
		if !got && strings.Contains(stderr.String(), "Timeout") {
			// The host stopped answering; later roots would time out too.
			if !answered {
				return "", errors.New("no response from " + host)
			}
			break
		}
		if !got && !answered && runErr != nil {
			return "", fmt.Errorf("snmpwalk failed: %s", firstLine(stderr.String(), runErr))
		}
	}
	if !answered {
		if ctx.Err() != nil {
			return "", errors.New("no response from " + host + " in 12 seconds")
		}
		return "", errors.New("no response from " + host)
	}
	return out.String(), nil
}

func firstLine(s string, fallback error) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback.Error()
	}
	line, _, _ := strings.Cut(s, "\n")
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}

// SNMPCheckAccessories walks the printer over SNMP and reports its hardware.
func (a *CLIAdapter) SNMPCheckAccessories(name, host, community string) (*SNMPInventory, error) {
	return a.accessories().SNMPCheck(name, host, community)
}

// GetAccessoryFeatures returns the optional accessory features.
func (a *CLIAdapter) GetAccessoryFeatures() (*AccessoryFeatureStatus, error) {
	return a.accessories().SNMPFeatures(), nil
}

// SetAccessoryFeatures turns optional accessory features on or off.
func (a *CLIAdapter) SetAccessoryFeatures(f AccessoryFeatures) (*AccessoryFeatureStatus, error) {
	return a.accessories().SetSNMPFeatures(f)
}

// GetPrinterInventory reports the imported print server list.
func (a *CLIAdapter) GetPrinterInventory() (*InventoryStatus, error) {
	return a.accessories().PrinterInventory(), nil
}

// ImportPrinterInventory replaces the print server list with a CSV export.
func (a *CLIAdapter) ImportPrinterInventory(sourceName string, data []byte) (*InventoryImportResult, error) {
	return a.accessories().ImportPrinterInventory(sourceName, data)
}

// RemovePrinterInventory forgets the print server list.
func (a *CLIAdapter) RemovePrinterInventory() (*InventoryStatus, error) {
	return a.accessories().RemovePrinterInventory()
}
