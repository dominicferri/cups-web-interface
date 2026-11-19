package cups

import (
	"bufio"
	"debug/macho"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PPDPathsToSearch lists default CUPS PPD directories on macOS and UNIX.
var PPDPathsToSearch = []string{
	"/private/etc/cups/ppd",
	"/etc/cups/ppd",
}

// FilterPathsToSearch lists standard directories where CUPS filter binaries reside.
var FilterPathsToSearch = []string{
	"/usr/libexec/cups/filter",
	"/usr/lib/cups/filter",
	"/opt/homebrew/lib/cups/filter",
	"/usr/local/lib/cups/filter",
	"/Library/Printers",
}

// InspectPrinterArch examines the printer's PPD file, extracts all declared filters/backends,
// inspects their Mach-O binary architectures, and evaluates Rosetta 2 dependency.
func InspectPrinterArch(printerName string) (*ArchAuditResult, error) {
	ppdPath := FindPPDFile(printerName)
	if ppdPath == "" {
		// No PPD file found; queue is either raw or driverless IPP Everywhere proxy
		return &ArchAuditResult{
			PrinterName:     printerName,
			PPDPath:         "",
			DriverModel:     "Driverless IPP Everywhere / System Proxy",
			Status:          "driverless",
			Label:           "Driverless (AirPrint / IPP)",
			RosettaRequired: false,
			Binaries:        []BinaryInfo{},
			MigrationAdvice: "This queue does not use a local PPD or vendor driver. It uses driverless IPP Everywhere or native system rasterizers, making it 100% future-proof without Rosetta 2.",
		}, nil
	}

	result, err := ParseAndInspectPPD(printerName, ppdPath)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect PPD %s: %w", ppdPath, err)
	}

	return result, nil
}

// FindPPDFile looks for <printerName>.ppd in standard search paths.
func FindPPDFile(printerName string) string {
	if !IsValidPrinterName(printerName) {
		return ""
	}
	for _, dir := range PPDPathsToSearch {
		path := filepath.Join(dir, printerName+".ppd")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// ParseAndInspectPPD extracts driver metadata and filters from a PPD file.
func ParseAndInspectPPD(printerName, ppdPath string) (*ArchAuditResult, error) {
	file, err := os.Open(ppdPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var driverModel string
	filterMap := make(map[string]bool)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "*NickName:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				driverModel = strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
		} else if strings.HasPrefix(line, "*ModelName:") && driverModel == "" {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				driverModel = strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
		} else if strings.HasPrefix(line, "*cupsFilter:") || strings.HasPrefix(line, "*cupsFilter2:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
				tokens := strings.Fields(val)
				if len(tokens) > 0 {
					binName := tokens[len(tokens)-1]
					if binName != "-" && binName != "" {
						filterMap[binName] = true
					}
				}
			}
		}
	}

	if driverModel == "" {
		driverModel = "PostScript Printer Driver (PPD)"
	}

	var binaries []BinaryInfo
	hasRosetta := false
	hasUniversal := false
	hasMissing := false
	hasNativeArm := false

	for binName := range filterMap {
		info := InspectBinary(binName)
		binaries = append(binaries, info)

		if !info.Exists {
			hasMissing = true
		} else if info.RosettaRequired {
			hasRosetta = true
		} else if info.IsUniversal {
			hasUniversal = true
		} else if info.IsNativeApple {
			hasNativeArm = true
		}
	}

	result := &ArchAuditResult{
		PrinterName: printerName,
		PPDPath:     ppdPath,
		DriverModel: driverModel,
		Binaries:    binaries,
	}

	if len(binaries) == 0 {
		result.Status = "driverless"
		result.Label = "Driverless (AirPrint / IPP)"
		result.RosettaRequired = false
		result.MigrationAdvice = "Queue uses driverless IPP Everywhere / AirPrint with native system filters. Zero vendor binaries are executed, making this queue 100% future-proof."
	} else if hasRosetta {
		result.Status = "rosetta_dependent"
		result.Label = "Intel Only (Rosetta 2)"
		result.RosettaRequired = true
		result.MigrationAdvice = "This printer queue relies on Intel x86_64 driver binaries running through Rosetta 2 translation. When Apple removes Rosetta 2 in a future macOS release, this queue will fail. Recommendation: Obtain updated Apple Silicon drivers from the manufacturer or reconfigure this queue with Driverless IPP Everywhere."
	} else if hasMissing {
		result.Status = "missing_binary"
		result.Label = "Missing Driver Binary"
		result.RosettaRequired = false
		result.MigrationAdvice = "One or more driver filter binaries referenced in the PPD could not be located on the filesystem. Document printing on this queue may encounter 'Filter failed' crashes."
	} else if hasUniversal || hasNativeArm {
		result.Status = "universal"
		result.Label = "Universal (Apple Silicon Ready)"
		result.RosettaRequired = false
		result.MigrationAdvice = "All filter and backend executables include native Apple Silicon (arm64) slices. This queue will continue operating seamlessly in a world without Rosetta 2."
	} else {
		result.Status = "driverless"
		result.Label = "Driverless / System Component"
		result.RosettaRequired = false
		result.MigrationAdvice = "Queue relies on standard system components."
	}

	return result, nil
}

// InspectBinary resolves the binary location and inspects its Mach-O slices.
func InspectBinary(binRef string) BinaryInfo {
	info := BinaryInfo{
		Name: filepath.Base(binRef),
		Type: "filter",
	}

	resolvedPath := resolveBinaryPath(binRef)
	info.Path = resolvedPath

	stat, err := os.Stat(resolvedPath)
	if err != nil || resolvedPath == "" {
		info.Exists = false
		info.Recommendation = "Binary missing on disk. Check driver installation."
		return info
	}

	info.Exists = true
	info.SizeBytes = stat.Size()

	slices, isScript := inspectMachOSlices(resolvedPath)
	if isScript {
		info.Slices = []string{"script (universal interpreter)"}
		info.IsUniversal = true
		info.IsNativeApple = true
		info.RosettaRequired = false
		info.Recommendation = "Shell or scripting wrapper executed by native system interpreter."
		return info
	}

	info.Slices = slices

	hasArm64 := false
	hasX86 := false

	for _, s := range slices {
		if strings.Contains(s, "arm64") {
			hasArm64 = true
		}
		if strings.Contains(s, "x86_64") || strings.Contains(s, "i386") {
			hasX86 = true
		}
	}

	if hasArm64 && hasX86 {
		info.IsUniversal = true
		info.IsNativeApple = true
		info.RosettaRequired = false
		info.Recommendation = "Universal Mach-O binary containing both Apple Silicon and Intel slices. Ready for macOS without Rosetta 2."
	} else if hasArm64 && !hasX86 {
		info.IsNativeApple = true
		info.RosettaRequired = false
		info.Recommendation = "Pure Apple Silicon binary. Ready for macOS without Rosetta 2."
	} else if hasX86 && !hasArm64 {
		info.RosettaRequired = true
		info.Recommendation = "Intel x86_64 binary requiring Rosetta 2 dynamic translation. Update driver to universal version before Rosetta 2 is discontinued."
	} else {
		// Non-Mach-O or fallback
		info.IsUniversal = true
		info.Recommendation = "System binary."
	}

	return info
}

func resolveBinaryPath(binRef string) string {
	if filepath.IsAbs(binRef) {
		if _, err := os.Stat(binRef); err == nil {
			return binRef
		}
	}

	// Search standard filter directories
	for _, dir := range FilterPathsToSearch {
		p := filepath.Join(dir, binRef)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try searching recursively in /Library/Printers if not found yet
	var foundPath string
	_ = filepath.Walk("/Library/Printers", func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && fi.Name() == filepath.Base(binRef) {
			foundPath = p
			return filepath.SkipAll
		}
		return nil
	})
	if foundPath != "" {
		return foundPath
	}

	return binRef
}

// inspectMachOSlices opens the binary using debug/macho to extract CPU slices.
func inspectMachOSlices(path string) ([]string, bool) {
	// Check if file is a script
	f, err := os.Open(path)
	if err == nil {
		var header [2]byte
		n, _ := f.Read(header[:])
		f.Close()
		if n == 2 && header[0] == '#' && header[1] == '!' {
			return nil, true
		}
	}

	var slices []string

	// 1. Try fat/universal Mach-O
	fat, err := macho.OpenFat(path)
	if err == nil {
		defer fat.Close()
		for _, arch := range fat.Arches {
			sliceName := cpuToString(arch.Cpu, arch.SubCpu)
			slices = append(slices, sliceName)
		}
		return slices, false
	}

	// 2. Try single-slice Mach-O
	m, err := macho.Open(path)
	if err == nil {
		defer m.Close()
		slices = append(slices, cpuToString(m.Cpu, m.SubCpu))
		return slices, false
	}

	// 3. Fallback to /usr/bin/lipo -archs if available on macOS
	if out, err := exec.Command("/usr/bin/lipo", "-archs", path).Output(); err == nil {
		fields := strings.Fields(strings.TrimSpace(string(out)))
		if len(fields) > 0 {
			return fields, false
		}
	}

	return []string{"unknown"}, false
}

func cpuToString(cpu macho.Cpu, subcpu uint32) string {
	switch cpu {
	case macho.CpuArm64:
		if subcpu == 2 {
			return "arm64e"
		}
		return "arm64"
	case macho.CpuAmd64:
		return "x86_64"
	case macho.Cpu386:
		return "i386"
	case macho.CpuArm:
		return "arm"
	default:
		return fmt.Sprintf("cpu-%d", cpu)
	}
}
