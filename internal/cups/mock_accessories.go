package cups

import (
	"fmt"
	"strings"
)

// mockAccessorySource builds PPD text from the mock printers' accessory
// options, so mock mode exercises the same parsing and provenance code as the
// live adapter. The queue PPD uses each option's CurrentValue as its default;
// the driver PPD uses DefaultValue.
type mockAccessorySource struct {
	m *MockAdapter
}

func (s *mockAccessorySource) buildPPD(name string, driver bool) ([]byte, error) {
	s.m.mu.RLock()
	defer s.m.mu.RUnlock()
	p, ok := s.m.printers[name]
	if !ok {
		return nil, fmt.Errorf("printer %q not found", name)
	}
	var b strings.Builder
	b.WriteString("*PPD-Adobe: \"4.3\"\n")
	fmt.Fprintf(&b, "*NickName: \"%s\"\n", p.Info)
	if p.ArchReadiness != nil && p.ArchReadiness.Status == "driverless" {
		b.WriteString("*APAirPrint: True\n")
	}
	if opts, ok := s.m.options[name]; ok && len(opts.Accessories) > 0 {
		b.WriteString("*OpenGroup: InstallableOptions/Installable Options\n")
		for _, o := range opts.Accessories {
			fmt.Fprintf(&b, "*OpenUI *%s/%s: PickOne\n", o.Key, o.DisplayName)
			def := o.CurrentValue
			if driver || def == "" {
				def = o.DefaultValue
			}
			fmt.Fprintf(&b, "*Default%s: %s\n", o.Key, def)
			for _, c := range o.Choices {
				fmt.Fprintf(&b, "*%s %s/%s: \"\"\n", o.Key, c.Name, c.Label)
			}
			fmt.Fprintf(&b, "*CloseUI: *%s\n", o.Key)
		}
		b.WriteString("*CloseGroup: InstallableOptions\n")
	}
	return []byte(b.String()), nil
}

func (s *mockAccessorySource) QueuePPD(name string) ([]byte, error) {
	return s.buildPPD(name, false)
}

func (s *mockAccessorySource) DriverPPD(nickName string) (string, []byte, error) {
	s.m.mu.RLock()
	var name string
	for _, p := range s.m.printers {
		if p.Info == nickName {
			name = p.Name
			break
		}
	}
	s.m.mu.RUnlock()
	if name == "" {
		return "", nil, fmt.Errorf("driver %q not found", nickName)
	}
	data, err := s.buildPPD(name, true)
	return "mock:///" + name + ".ppd", data, err
}

func (s *mockAccessorySource) DeviceURI(name string) (string, error) {
	s.m.mu.RLock()
	defer s.m.mu.RUnlock()
	p, ok := s.m.printers[name]
	if !ok {
		return "", fmt.Errorf("printer %q not found", name)
	}
	return p.URI, nil
}

func (s *mockAccessorySource) ApplyOptions(name string, options map[string]string) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	opts, ok := s.m.options[name]
	if !ok {
		return fmt.Errorf("printer %q not found", name)
	}
	for k, v := range options {
		for i := range opts.Accessories {
			if opts.Accessories[i].Key == k {
				opts.Accessories[i].CurrentValue = v
			}
		}
	}
	return nil
}

func (s *mockAccessorySource) PrinterAttributes(uri string) (string, error) {
	return "", fmt.Errorf("mock mode does not query printers")
}

// mockSNMPWalk is sample snmpwalk -On output for mock mode: two drawers, a
// bypass tray, a finisher with a stapler and a hole punch, and duplex.
const mockSNMPWalk = `.1.3.6.1.2.1.1.1.0 = STRING: "Mock printer (sample SNMP data)"
.1.3.6.1.2.1.1.5.0 = STRING: "mock-printer"
.1.3.6.1.2.1.25.3.2.1.3.1 = STRING: "Mock Laser 5000"
.1.3.6.1.2.1.25.3.2.1.3.2 = STRING: "Office Finisher"
.1.3.6.1.2.1.43.8.2.1.2.1.1 = INTEGER: 3
.1.3.6.1.2.1.43.8.2.1.2.1.2 = INTEGER: 3
.1.3.6.1.2.1.43.8.2.1.2.1.3 = INTEGER: 5
.1.3.6.1.2.1.43.8.2.1.9.1.1 = INTEGER: 520
.1.3.6.1.2.1.43.8.2.1.9.1.2 = INTEGER: 520
.1.3.6.1.2.1.43.8.2.1.9.1.3 = INTEGER: 100
.1.3.6.1.2.1.43.8.2.1.13.1.1 = STRING: "Tray 1"
.1.3.6.1.2.1.43.8.2.1.13.1.2 = STRING: "Tray 2"
.1.3.6.1.2.1.43.8.2.1.13.1.3 = STRING: "Bypass Tray"
.1.3.6.1.2.1.43.9.2.1.2.1.1 = INTEGER: 4
.1.3.6.1.2.1.43.9.2.1.4.1.1 = INTEGER: 250
.1.3.6.1.2.1.43.9.2.1.7.1.1 = STRING: "Center Tray"
.1.3.6.1.2.1.43.13.4.1.9.1.1 = INTEGER: 3
.1.3.6.1.2.1.43.30.1.1.2.2.1 = INTEGER: 3
.1.3.6.1.2.1.43.30.1.1.3.2.1 = INTEGER: 3
.1.3.6.1.2.1.43.30.1.1.10.2.1 = STRING: "Stapler"
.1.3.6.1.2.1.43.30.1.1.2.2.2 = INTEGER: 8
.1.3.6.1.2.1.43.30.1.1.3.2.2 = INTEGER: 3
.1.3.6.1.2.1.43.30.1.1.10.2.2 = STRING: "2/3 Hole Punch"
`

func (s *mockAccessorySource) SNMPWalk(host, community string, roots []string) (string, error) {
	return mockSNMPWalk, nil
}

func (s *mockAccessorySource) SNMPAvailable() bool { return true }

func (s *mockAccessorySource) QueueUUID(name string) (string, error) {
	return "urn:uuid:mock-" + strings.ToLower(name), nil
}

func (s *mockAccessorySource) QueueNames() ([]string, error) {
	s.m.mu.RLock()
	defer s.m.mu.RUnlock()
	names := make([]string, 0, len(s.m.printers))
	for n := range s.m.printers {
		names = append(names, n)
	}
	return names, nil
}

func (s *mockAccessorySource) IsAdmin() bool    { return true }
func (s *mockAccessorySource) UserName() string { return "mock-admin" }

func (m *MockAdapter) accessories() *AccessoryManager {
	m.accOnce.Do(func() {
		if m.Accessories == nil {
			m.Accessories = NewAccessoryManager(&mockAccessorySource{m: m}, NewMemoryOverrideStore())
		}
	})
	return m.Accessories
}

func (m *MockAdapter) GetAccessories(name string) (*AccessoryReport, error) {
	return m.accessories().Report(name)
}

func (m *MockAdapter) SetAccessories(name string, values map[string]string, acknowledged bool) (*AccessoryReport, error) {
	return m.accessories().Set(name, values, acknowledged)
}

func (m *MockAdapter) ResetAccessories(name string) (*AccessoryReport, error) {
	return m.accessories().Reset(name)
}

func (m *MockAdapter) SuggestAccessories(name string) (*AccessorySuggestions, error) {
	return m.accessories().Suggest(name)
}

func (m *MockAdapter) SNMPCheckAccessories(name, host, community string) (*SNMPInventory, error) {
	return m.accessories().SNMPCheck(name, host, community)
}

func (m *MockAdapter) GetAccessoryFeatures() (*AccessoryFeatureStatus, error) {
	return m.accessories().SNMPFeatures(), nil
}

func (m *MockAdapter) SetAccessoryFeatures(f AccessoryFeatures) (*AccessoryFeatureStatus, error) {
	return m.accessories().SetSNMPFeatures(f)
}
