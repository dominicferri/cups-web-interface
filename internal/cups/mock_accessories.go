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
