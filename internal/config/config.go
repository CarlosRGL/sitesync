package config

// Config is the top-level structure for a sitesync TOML config file.
type Config struct {
	Site        SiteConfig      `toml:"site"`
	Source      SourceConfig    `toml:"source"`
	Destination DestConfig      `toml:"destination"`
	Database    DatabaseConfig  `toml:"database"`
	Replace     []ReplacePair   `toml:"replace"`
	Sync        []SyncPair      `toml:"sync"`
	Transport   TransportConfig `toml:"transport"`
	Hooks       HooksConfig     `toml:"hooks"`
	Logging     LoggingConfig   `toml:"logging"`

	// configFilePath is set by the loader and not serialised.
	configFilePath string
}

// ConfigFilePath returns the path used to load this config.
func (c *Config) ConfigFilePath() string { return c.configFilePath }

// SiteConfig holds display metadata for the site.
type SiteConfig struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

// SourceConfig holds all settings for the remote (source) side.
type SourceConfig struct {
	// Connection
	Server string `toml:"server"`
	User   string `toml:"user"`
	Port   int    `toml:"port"`

	// Dump source type: remote_base | local_base | remote_file | local_file
	Type     string `toml:"type"`
	File     string `toml:"file"`
	Compress bool   `toml:"compress"`

	// Source DB credentials
	DBHostname string `toml:"db_hostname"`
	DBPort     string `toml:"db_port"`
	DBName     string `toml:"db_name"`
	DBUser     string `toml:"db_user"`
	DBPassword string `toml:"db_password"`

	// Site URL helpers
	SiteProtocol string `toml:"site_protocol"`
	SiteHost     string `toml:"site_host"`
	SiteSlug     string `toml:"site_slug"`
	FilesRoot    string `toml:"files_root"`

	// Remote tool paths
	PathToMysqldump string `toml:"path_to_mysqldump"`
	RemoteNice      string `toml:"remote_nice"`
}

// DestConfig holds all settings for the local (destination) side.
type DestConfig struct {
	// Site URL helpers
	SiteProtocol string `toml:"site_protocol"`
	SiteHost     string `toml:"site_host"`
	SiteSlug     string `toml:"site_slug"`
	FilesRoot    string `toml:"files_root"`

	// Destination DB credentials
	DBHostname string `toml:"db_hostname"`
	DBPort     string `toml:"db_port"`
	DBName     string `toml:"db_name"`
	DBUser     string `toml:"db_user"`
	DBPassword string `toml:"db_password"`

	// Local tool paths
	PathToMySQL     string `toml:"path_to_mysql"`
	PathToMysqldump string `toml:"path_to_mysqldump"`
	PathToRsync     string `toml:"path_to_rsync"`
	PathToLftp      string `toml:"path_to_lftp"`
	LocalNice       string `toml:"local_nice"`
}

// DatabaseConfig holds mysqldump / import options.
type DatabaseConfig struct {
	SQLOptionsStructure string   `toml:"sql_options_structure"`
	SQLOptionsExtra     string   `toml:"sql_options_extra"`
	IgnoreTables        []string `toml:"ignore_tables"`
}

// ReplacePair is one find/replace entry applied to the SQL dump.
type ReplacePair struct {
	Search  string `toml:"search"`
	Replace string `toml:"replace"`
}

// SyncPair is one source→destination directory pair for file sync.
type SyncPair struct {
	Src string `toml:"src"`
	Dst string `toml:"dst"`
}

// TransportConfig controls how files are transferred.
type TransportConfig struct {
	Type         string     `toml:"type"` // rsync | lftp
	RsyncOptions string     `toml:"rsync_options"`
	Exclude      []string   `toml:"exclude"`
	LFTP         LFTPConfig `toml:"lftp"`
}

// LFTPConfig holds lftp-specific options.
type LFTPConfig struct {
	Password       string `toml:"password"`
	Port           int    `toml:"port"`
	ConnectOptions string `toml:"connect_options"`
	MirrorOptions  string `toml:"mirror_options"`
}

// HooksConfig specifies where hook scripts are located.
type HooksConfig struct {
	// Path is relative to the directory containing the config file.
	Path string `toml:"path"`
}

// LoggingConfig specifies the log file path.
type LoggingConfig struct {
	File string `toml:"file"`
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Source: SourceConfig{
			Port:            22,
			Type:            "remote_base",
			Compress:        true,
			SiteProtocol:    "http://",
			PathToMysqldump: "mysqldump",
		},
		Destination: DestConfig{
			SiteProtocol:    "http://",
			DBHostname:      "localhost",
			PathToMySQL:     "mysql",
			PathToMysqldump: "mysqldump",
			PathToRsync:     "rsync",
			PathToLftp:      "lftp",
		},
		Database: DatabaseConfig{
			SQLOptionsStructure: "--default-character-set=utf8",
		},
		Transport: TransportConfig{
			Type:         "rsync",
			RsyncOptions: "-uvrpztl",
			Exclude:      []string{"/sitesync/", ".git/", ".svn/", ".DS_Store"},
			LFTP: LFTPConfig{
				Port:          21,
				MirrorOptions: "--parallel=16 --verbose --only-newer",
			},
		},
		Hooks: HooksConfig{
			Path: "hook",
		},
		Logging: LoggingConfig{
			File: "log/sitesync.log",
		},
	}
}
