package plugin

import pub "github.com/codyconfer/mino/plugin"

type Credential = pub.Credential
type CredentialStore = pub.CredentialStore
type Host = pub.Host

type LoginField = pub.LoginField
type LoginProvider = pub.LoginProvider

type BackupDestination = pub.BackupDestination
type BackupDestinationFunc = pub.BackupDestinationFunc

const (
	KindLogin  = pub.KindLogin
	KindBackup = pub.KindBackup
)

func HostOf(bc BuildContext) (Host, bool) { return pub.HostOf(bc) }

func RegisterLoginProvider(p LoginProvider) { pub.RegisterLoginProvider(p) }

func LoginProviders() []LoginProvider { return pub.LoginProviders() }

func LookupLoginProvider(key string) (LoginProvider, bool) { return pub.LookupLoginProvider(key) }

func RegisterBackupDestination(pluginID, name string, open BackupDestinationFunc) {
	pub.RegisterBackupDestination(pluginID, name, open)
}

func LookupBackupDestination(name string) (BackupDestinationFunc, bool) {
	return pub.LookupBackupDestination(name)
}

func BackupDestinations() []string { return pub.BackupDestinations() }

type ParamSpec = pub.ParamSpec

func RegisterQueryParams(signal string, specs ...ParamSpec) {
	pub.RegisterQueryParams(signal, specs...)
}

func QueryParams(signal string) []ParamSpec { return pub.QueryParams(signal) }

func ParamSignals() []string { return pub.ParamSignals() }

func Setting(settings map[string]any, key, def string) string { return pub.Setting(settings, key, def) }

func SettingInt(settings map[string]any, key string, def int) int {
	return pub.SettingInt(settings, key, def)
}

func SettingBool(settings map[string]any, key string, def bool) bool {
	return pub.SettingBool(settings, key, def)
}

func SettingList(settings map[string]any, key string) []string { return pub.SettingList(settings, key) }

func ResetLoginProviders() { pub.ResetLoginProviders() }

func ResetBackupDestinations() { pub.ResetBackupDestinations() }

func ResetQueryParams() { pub.ResetQueryParams() }
