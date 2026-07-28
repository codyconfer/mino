package config

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/configdb"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
)

type ReconcilePolicy int

const (
	ReconcilePrompt ReconcilePolicy = iota
	ReconcileApply
	ReconcileSession
	ReconcileIgnore
)

func ReconcilePolicyNames() []string {
	return []string{"prompt", "apply", "session", "ignore"}
}

func ParseReconcilePolicy(s string) (ReconcilePolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "prompt", "ask":
		return ReconcilePrompt, nil
	case "apply", "import":
		return ReconcileApply, nil
	case "session", "file":
		return ReconcileSession, nil
	case "ignore", "stored", "db", "duckdb":
		return ReconcileIgnore, nil
	}
	return ReconcilePrompt, errs.Newf(errs.KindUsage, "unknown reconcile policy %q: want one of %v", s, ReconcilePolicyNames())
}

type stagedDirective struct {
	name    string
	content []byte
	format  string
	hasFile bool
	rec     sisyphus.Reconciliation
	pending bool
}

func LoadConfigAndDirectives(homeOverride, configFile string, policy ReconcilePolicy, interactive bool, in io.Reader, out io.Writer) (*Config, *Directives, *sisyphus.Manager, error) {
	home, err := Home(homeOverride)
	if err != nil {
		return nil, nil, nil, err
	}
	gs := LoadGlobalSettings()
	res := &Resolver{home: home, preferDB: gs.PreferDB, policy: policy, interactive: interactive, in: in, out: out}

	var mgr *sisyphus.Manager
	if m, err := OpenStore(context.Background(), home); err != nil {
		log.Debugf("store DB unavailable: %v; reading from files", err)
	} else {
		mgr = m
	}

	if configFile != "" {
		raw, format, err := sconfig.ReadFileAt(configFile)
		if err != nil {
			return nil, nil, nil, err
		}
		log.Debugf("using session config file %s (not persisted)", configFile)
		cfg, err := ParseConfig(home, raw, format)
		if err != nil {
			return nil, nil, nil, err
		}
		directives, err := loadDirectives(mgr, res, home)
		if err != nil {
			return nil, nil, nil, err
		}
		return cfg, directives, mgr, nil
	}

	if mgr == nil {
		cfg, err := Load(homeOverride)
		if err != nil {
			return nil, nil, nil, err
		}
		directives, err := LoadDirectivesFromFiles(home)
		if err != nil {
			return nil, nil, nil, err
		}
		return cfg, directives, nil, nil
	}

	staged, err := collectStaged(context.Background(), mgr, home, homeOverride)
	if err != nil {
		return nil, nil, nil, err
	}
	resolver, err := res.resolverFor(staged)
	if err != nil {
		return nil, nil, nil, err
	}

	byName := map[string]stagedDirective{}
	for _, st := range staged {
		byName[st.name] = st
	}
	cfg, err := applyConfigStage(context.Background(), mgr, resolver, home, byName[ConfigDirective])
	if err != nil {
		return nil, nil, nil, err
	}
	var q, fl, roles []byte
	if q, err = applyCollectionStage(context.Background(), mgr, resolver, byName[DirQueries]); err != nil {
		return nil, nil, nil, err
	}
	if fl, err = applyCollectionStage(context.Background(), mgr, resolver, byName[DirFlights]); err != nil {
		return nil, nil, nil, err
	}
	if roles, err = applyCollectionStage(context.Background(), mgr, resolver, byName[KindRoles]); err != nil {
		return nil, nil, nil, err
	}
	directives, err := NewDirectives(q, fl, roles)
	if err != nil {
		return nil, nil, nil, err
	}
	return cfg, directives, mgr, nil
}

func loadDirectives(mgr *sisyphus.Manager, res *Resolver, home string) (*Directives, error) {
	if mgr == nil {
		return LoadDirectivesFromFiles(home)
	}
	var q, fl, r []byte
	var err error
	if q, err = reconcileCollection(mgr, res, home, DirQueries); err != nil {
		return nil, err
	}
	if fl, err = reconcileCollection(mgr, res, home, DirFlights); err != nil {
		return nil, err
	}
	if r, err = reconcileCollection(mgr, res, home, KindRoles); err != nil {
		return nil, err
	}
	return NewDirectives(q, fl, r)
}

func ReloadDirectives(mgr *sisyphus.Manager, home string, policy ReconcilePolicy) (*Directives, error) {
	if mgr == nil {
		return LoadDirectivesFromFiles(home)
	}
	gs := LoadGlobalSettings()
	res := &Resolver{
		home:        home,
		preferDB:    gs.PreferDB,
		policy:      policy,
		interactive: false,
	}
	return loadDirectives(mgr, res, home)
}

func collectStaged(ctx context.Context, mgr *sisyphus.Manager, home, homeOverride string) ([]stagedDirective, error) {
	_, raw, format, err := ReadConfigFile(homeOverride)
	if err != nil {
		return nil, err
	}
	out := make([]stagedDirective, 0, 1+len(CollectionDirectives()))
	cfgStage, err := stageOne(ctx, mgr, ConfigDirective, raw, format, len(raw) > 0)
	if err != nil {
		return nil, err
	}
	out = append(out, cfgStage)
	for _, name := range CollectionDirectives() {
		blob, has, err := SerializeCollection(home, name)
		if err != nil {
			return nil, err
		}
		st, err := stageOne(ctx, mgr, name, blob, "collection", has)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func stageOne(ctx context.Context, mgr *sisyphus.Manager, name string, content []byte, format string, hasFile bool) (stagedDirective, error) {
	st := stagedDirective{name: name, content: content, format: format, hasFile: hasFile}
	rec, pending, err := pendingReconciliation(ctx, mgr, name, content, format, hasFile)
	if err != nil {
		return st, err
	}
	st.rec = rec
	st.pending = pending
	return st, nil
}

func pendingReconciliation(ctx context.Context, mgr *sisyphus.Manager, name string, content []byte, format string, hasFile bool) (sisyphus.Reconciliation, bool, error) {
	if mgr == nil || mgr.Mode() != sisyphus.ModeBoth || !hasFile {
		return sisyphus.Reconciliation{}, false, nil
	}
	cur, hasCur, err := mgr.Current(ctx, name)
	if err != nil {
		return sisyphus.Reconciliation{}, false, err
	}
	if !hasCur || cur.Hash == configdb.Hash(format, content) {
		return sisyphus.Reconciliation{}, false, nil
	}
	return sisyphus.Reconciliation{
		Name:        name,
		FileContent: content,
		FileFormat:  format,
		DB:          cur,
		HasDB:       hasCur,
	}, true, nil
}

func applyConfigStage(ctx context.Context, mgr *sisyphus.Manager, res sisyphus.Resolver, home string, st stagedDirective) (*Config, error) {
	eff, effFormat, err := mgr.Reconcile(ctx, st.name, st.content, st.format, st.hasFile, res)
	if err != nil {
		return nil, err
	}
	return ParseConfig(home, eff, effFormat)
}

func applyCollectionStage(ctx context.Context, mgr *sisyphus.Manager, res sisyphus.Resolver, st stagedDirective) ([]byte, error) {
	eff, _, err := mgr.Reconcile(ctx, st.name, st.content, st.format, st.hasFile, res)
	return eff, err
}

func reconcileCollection(mgr *sisyphus.Manager, res sisyphus.Resolver, home, name string) ([]byte, error) {
	blob, has, err := SerializeCollection(home, name)
	if err != nil {
		return nil, err
	}
	eff, _, err := mgr.Reconcile(context.Background(), name, blob, "collection", has, res)
	return eff, err
}

type Resolver struct {
	home        string
	preferDB    bool
	policy      ReconcilePolicy
	interactive bool
	in          io.Reader
	out         io.Writer
}

type batchActionResolver struct {
	act      sisyphus.Action
	names    map[string]bool
	fallback sisyphus.Resolver
}

func (b batchActionResolver) Resolve(rec sisyphus.Reconciliation) (sisyphus.Action, error) {
	if b.names[rec.Name] {
		return b.act, nil
	}
	return b.fallback.Resolve(rec)
}

func (r *Resolver) resolverFor(staged []stagedDirective) (sisyphus.Resolver, error) {
	pending := make([]sisyphus.Reconciliation, 0, len(staged))
	names := map[string]bool{}
	for _, st := range staged {
		if st.pending {
			pending = append(pending, st.rec)
			names[st.rec.Name] = true
		}
	}
	if len(pending) == 0 {
		return r, nil
	}
	batch := func(act sisyphus.Action) sisyphus.Resolver {
		return batchActionResolver{act: act, names: names, fallback: r}
	}
	switch r.policy {
	case ReconcileApply:
		log.Infof("applied staged changes to the store (%s)", joinNames(pending))
		return batch(sisyphus.ActionImport), nil
	case ReconcileSession:
		log.Debugf("using staged changes for this session (%s)", joinNames(pending))
		return batch(sisyphus.ActionUseFile), nil
	case ReconcileIgnore:
		log.Debugf("staged changes ignored; using the stored version (%s)", joinNames(pending))
		return batch(sisyphus.ActionUseDB), nil
	}
	if !r.interactive {
		if r.preferDB {
			log.Warnf("staged changes ignored; using the stored version (prefer_duckdb): %s", joinNames(pending))
			return batch(sisyphus.ActionUseDB), nil
		}
		log.Warnf("staged changes used for this session only; run `munin apply` to write them to the store: %s", joinNames(pending))
		return batch(sisyphus.ActionUseFile), nil
	}
	act, err := r.promptAll(pending)
	if err != nil {
		return nil, err
	}
	return batch(act), nil
}

func joinNames(recs []sisyphus.Reconciliation) string {
	names := make([]string, len(recs))
	for i, rec := range recs {
		names[i] = rec.Name
	}
	return strings.Join(names, ", ")
}

func (r *Resolver) Resolve(rec sisyphus.Reconciliation) (sisyphus.Action, error) {
	if !rec.HasDB {
		return sisyphus.ActionImport, nil
	}
	switch r.policy {
	case ReconcileApply:
		log.Infof("%s: applied staged changes to the store", rec.Name)
		return sisyphus.ActionImport, nil
	case ReconcileSession:
		log.Debugf("%s: using staged changes for this session", rec.Name)
		return sisyphus.ActionUseFile, nil
	case ReconcileIgnore:
		log.Debugf("%s: staged changes ignored; using the stored version", rec.Name)
		return sisyphus.ActionUseDB, nil
	}
	if !r.interactive {
		if r.preferDB {
			log.Warnf("%s: staged changes ignored; using the stored version (prefer_duckdb)", rec.Name)
			return sisyphus.ActionUseDB, nil
		}
		log.Warnf("%s: staged changes used for this session only; run `munin apply %s` to write them to the store", rec.Name, rec.Name)
		return sisyphus.ActionUseFile, nil
	}
	return r.promptAll([]sisyphus.Reconciliation{rec})
}

func (r *Resolver) promptAll(recs []sisyphus.Reconciliation) (sisyphus.Action, error) {
	out := r.out
	in := bufio.NewReader(r.in)
	for {
		fmt.Fprint(out, "\n"+renderReconcileBatchPanel(out, recs)+"\n")
		fmt.Fprint(out, reconcilePromptLine())
		key, err := readPromptKey(r.in, in)
		if err != nil && key == "" {
			fmt.Fprintln(out)
			return sisyphus.ActionUseFile, nil
		}
		echoPromptKey(out, key)
		switch reconcileChoiceFor(key) {
		case choiceApply:
			return sisyphus.ActionImport, nil
		case choiceSession:
			return sisyphus.ActionUseFile, nil
		case choiceIgnore:
			return sisyphus.ActionUseDB, nil
		case choiceEdit:
			if r.home == "" {
				fmt.Fprintln(out, renderReconcileNotice("config home unknown; cannot open editor"))
				continue
			}
			if err := openConfigEditor(r.home); err != nil {
				fmt.Fprintln(out, renderReconcileNotice(err.Error()))
				continue
			}
			fmt.Fprintln(out, renderReconcileNotice("opened "+r.home+" in editor"))
		case choiceDiscard:
			ok, err := r.confirmDiscardAll(in, recs)
			if err != nil {
				return 0, err
			}
			if !ok {
				continue
			}
			return sisyphus.ActionUseDB, nil
		default:
			fmt.Fprintln(out, renderReconcileNotice("unrecognized choice"))
		}
	}
}

func (r *Resolver) confirmDiscardAll(in *bufio.Reader, recs []sisyphus.Reconciliation) (bool, error) {
	fmt.Fprint(r.out, discardConfirmBatchLine(r.home, recs))
	key, err := readPromptKey(r.in, in)
	if err != nil && key == "" {
		fmt.Fprintln(r.out)
		fmt.Fprintln(r.out, renderReconcileNotice("kept the staged files"))
		return false, nil
	}
	echoPromptKey(r.out, key)
	if strings.ToLower(key) != "y" {
		fmt.Fprintln(r.out, renderReconcileNotice("kept the staged files"))
		return false, nil
	}
	for _, rec := range recs {
		if err := deleteDirectiveFiles(r.home, rec.Name); err != nil {
			fmt.Fprintln(r.out, renderReconcileNotice("discard failed: "+err.Error()))
			return false, nil
		}
	}
	fmt.Fprintln(r.out, renderReconcileNotice("discarded staged "+joinNames(recs)+"; using the stored version"))
	return true, nil
}

func readPromptKey(raw io.Reader, in *bufio.Reader) (string, error) {
	restore := enterRawTerminal(raw)
	defer restore()
	b, err := in.ReadByte()
	if err != nil {
		return "", err
	}
	switch b {
	case '\r', '\n':
		return "", nil
	case 0x03:
		return "", io.EOF
	default:
		return string(b), nil
	}
}

func echoPromptKey(out io.Writer, key string) {
	if key == "" {
		fmt.Fprintln(out)
		return
	}
	fmt.Fprintln(out, key)
}

func enterRawTerminal(r io.Reader) (restore func()) {
	restore = func() {}
	type fd interface{ Fd() uintptr }
	f, ok := r.(fd)
	if !ok || !term.IsTerminal(f.Fd()) {
		return restore
	}
	state, err := term.MakeRaw(f.Fd())
	if err != nil {
		return restore
	}
	return func() { _ = term.Restore(f.Fd(), state) }
}

func deleteDirectiveFiles(home, name string) error {
	if name == ConfigDirective {
		_, err := sconfig.RemoveFiles(home, ConfigDirective, nil)
		return err
	}
	_, err := ClearCollection(home, name)
	return err
}
