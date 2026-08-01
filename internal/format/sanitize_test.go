package format

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/signals"
)

const (
	oscTitle    = "\x1b]0;pwned\x07"
	clearScreen = "\x1b[2J"
	fakeChip    = "\x1b[31mmerged\x1b[0m"
	delRaw      = "\x7f"
	c1CSI       = "\u009b"
	c1NEL       = "\u0085"
	forgedRow   = "\n- [FAKE](https://evil.invalid/1) forged row"
)

func hostileText(label string) string {
	return label + oscTitle + clearScreen + fakeChip + delRaw + c1CSI + c1NEL
}

const sinkTemplate = `# {{ .Name }} · {{ .Formatter }} · {{ .Kind }} · {{ .Role }}

{{ range .Queries }}## {{ .Title }} ({{ .Query }})
{{ range .Sections }}### {{ .Title }} [{{ .Signal }}] {{ meta "age" .Meta }}
{{ if .Err }}- error: {{ .Err }}
{{ end }}{{ range .Items }}- [{{ .Title }}]({{ .URL }}) {{ .Kind }} · {{ .Subtitle }} · @{{ .Meta.author }} · {{ .Meta.repo }}
{{ .Body }}
{{ end }}{{ end }}{{ end }}total: {{ .Count }}
errors: {{ join "; " .Errors }}
`

func hostileInput() Input {
	return Input{
		Formatter: hostileText("standup"),
		Name:      hostileText("morning"),
		Kind:      hostileText("flight"),
		Role:      hostileText("ic"),
		Now:       testNow,
		Groups: []InputGroup{{
			Query: hostileText("my-prs"),
			Title: hostileText("My PRs") + forgedRow,
			Sections: []signals.Section{
				{
					Signal: hostileText("github"),
					Title:  hostileText("Open PRs") + forgedRow,
					Meta:   map[string]string{"cache": "stale", hostileText("age"): hostileText("5m") + forgedRow},
					Items: []signals.Item{{
						Kind:      hostileText("pr"),
						Title:     hostileText("clamp the backoff") + forgedRow,
						Subtitle:  hostileText("acme/tools") + forgedRow,
						Body:      "## " + hostileText("summary") + "\n\nsecond line " + oscTitle,
						URL:       "https://x/1" + oscTitle + forgedRow,
						Timestamp: testNow.Add(-time.Hour),
						Meta: map[string]string{
							"author": hostileText("ada") + forgedRow,
							"repo":   hostileText("acme/tools"),
						},
					}},
				},
				{
					Signal: "jira",
					Title:  "Tickets",
					Err:    errors.New(hostileText("jira unreachable") + forgedRow),
				},
			},
		}},
	}
}

var controlChecks = []struct {
	name string
	seq  string
}{
	{"ESC (0x1b)", "\x1b"},
	{"BEL (0x07)", "\x07"},
	{"DEL (0x7f)", "\x7f"},
	{"CSI (U+009B)", c1CSI},
	{"NEL (U+0085)", c1NEL},
}

func assertNoControls(t *testing.T, sink, out string) {
	t.Helper()
	for _, bad := range controlChecks {
		if i := strings.Index(out, bad.seq); i >= 0 {
			t.Errorf("%s: %s survived at byte %d\n%q", sink, bad.name, i, out)
		}
	}
}

func assertNoForgedRow(t *testing.T, sink, out string) {
	t.Helper()
	for i, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- [FAKE]") {
			t.Errorf("%s: line %d is a forged report row: %q", sink, i, line)
		}
	}
}

func hostileReportText(t *testing.T) string {
	t.Helper()
	text, err := Render("sink", sinkTemplate, Build(hostileInput()))
	if err != nil {
		t.Fatalf("Render err = %v", err)
	}
	return text
}

func TestDeliverSinksCarryNoTerminalControls(t *testing.T) {
	text := hostileReportText(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	var suppressed, status strings.Builder
	var clip string
	if err := Deliver(&suppressed, &status, text, func(s string) error { clip = s; return nil }, path); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var stdout strings.Builder
	if err := Deliver(&stdout, nil, text, nil, ""); err != nil {
		t.Fatalf("Deliver stdout err = %v", err)
	}

	for _, sink := range []struct {
		name string
		out  string
	}{
		{"--out file", string(fileBytes)},
		{"clipboard", clip},
		{"stdout", stdout.String()},
	} {
		if sink.out == "" {
			t.Fatalf("%s received nothing", sink.name)
		}
		assertNoControls(t, sink.name, sink.out)
		assertNoForgedRow(t, sink.name, sink.out)
	}
}

func TestBuildSanitisesEveryStringItCopies(t *testing.T) {
	r := Build(hostileInput())
	walkStrings(t, reflect.ValueOf(r), "Report", func(path, s string) {
		for _, bad := range controlChecks {
			if strings.Contains(s, bad.seq) {
				t.Errorf("%s carries %s: %q", path, bad.name, s)
			}
		}
	})
}

func TestBuildKeepsBodyNewlinesAndFlattensSingleLineFields(t *testing.T) {
	r := Build(hostileInput())
	it := r.Items[0]

	if !strings.Contains(it.Body, "\nsecond line") {
		t.Errorf("Item.Body lost its newline: %q", it.Body)
	}
	for name, got := range map[string]string{
		"Kind": it.Kind, "Title": it.Title, "Subtitle": it.Subtitle, "URL": it.URL,
		"Meta[author]": it.Meta["author"], "Section.Title": r.Sections[0].Title,
		"Section.Signal": r.Sections[0].Signal, "Group.Title": r.Queries[0].Title,
		"Report.Name": r.Name, "Section.Err": r.Sections[1].Err, "Errors[0]": r.Errors[0],
	} {
		if strings.ContainsAny(got, "\n\r\t") {
			t.Errorf("%s is not a single line: %q", name, got)
		}
	}
	if !strings.Contains(it.Title, "FAKE") {
		t.Errorf("sanitising dropped visible text from Title: %q", it.Title)
	}
}

func TestBuildLeavesBenignInputUnchanged(t *testing.T) {
	in := fixtureInput()
	r := Build(in)
	if r.Name != in.Name || r.Formatter != in.Formatter || r.Kind != in.Kind || r.Role != in.Role {
		t.Errorf("header fields were rewritten: %+v", r)
	}
	if r.Sections[0].Title != "Open PRs" || r.Sections[0].Signal != "github" {
		t.Errorf("section fields were rewritten: %+v", r.Sections[0])
	}
	it := r.Items[0]
	src := in.Groups[0].Sections[0].Items[0]
	if it.Title != src.Title || it.URL != src.URL || it.Kind != src.Kind ||
		it.Subtitle != src.Subtitle || it.Body != src.Body {
		t.Errorf("item fields were rewritten: %+v", it)
	}
	if !reflect.DeepEqual(it.Meta, src.Meta) {
		t.Errorf("item meta was rewritten: %v, want %v", it.Meta, src.Meta)
	}
	if !reflect.DeepEqual(r.Sections[0].Meta, in.Groups[0].Sections[0].Meta) {
		t.Errorf("section meta was rewritten: %v", r.Sections[0].Meta)
	}
}

func TestBuildDoesNotMutateItsInput(t *testing.T) {
	in := hostileInput()
	Build(in)
	src := in.Groups[0].Sections[0]
	if !strings.Contains(src.Title, oscTitle) || !strings.Contains(src.Items[0].Title, oscTitle) ||
		!strings.Contains(src.Items[0].Meta["author"], oscTitle) {
		t.Error("Build mutated the caller's sections")
	}
}

func walkStrings(t *testing.T, v reflect.Value, path string, check func(path, s string)) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		check(path, v.String())
	case reflect.Struct:
		for i := range v.NumField() {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			walkStrings(t, v.Field(i), path+"."+v.Type().Field(i).Name, check)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			walkStrings(t, v.Index(i), path+"["+strconv.Itoa(i)+"]", check)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkStrings(t, k, path+" key", check)
			walkStrings(t, v.MapIndex(k), path+"["+k.String()+"]", check)
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			walkStrings(t, v.Elem(), path, check)
		}
	}
}
