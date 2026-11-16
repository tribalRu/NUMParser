package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/ilyakaznacheev/cleanenv"
	"gopkg.in/yaml.v3"
)

// const SaveReleasePath = "/home/yourok/numParser/releases"
const SaveReleasePath = "public/releases"
const ConfigFile = "config.yml"

var ProxyHost = ""
var UseProxy = false

type ReleasesConfig struct {
	MovieSort         string `yaml:"movie_sort" json:"movie_sort" env:"RELEASES_MOVIE_SORT" env-default:"release_date"`
	MovieMaxAgeMonths int    `yaml:"movie_max_age_months" json:"movie_max_age_months" env:"RELEASES_MOVIE_MAX_AGE_MONTHS" env-default:"6"`
	TVSort            string `yaml:"tv_sort" json:"tv_sort" env:"RELEASES_TV_SORT" env-default:"last_air_date"`
	TVMaxAgeMonths    int    `yaml:"tv_max_age_months" json:"tv_max_age_months" env:"RELEASES_TV_MAX_AGE_MONTHS" env-default:"6"`
	AnimeSort         string `yaml:"anime_sort" json:"anime_sort" env:"RELEASES_ANIME_SORT" env-default:"last_air_date"`
	AnimeMaxAgeMonths int    `yaml:"anime_max_age_months" json:"anime_max_age_months" env:"RELEASES_ANIME_MAX_AGE_MONTHS" env-default:"6"`
}

type SecurityConfig struct {
	EnableSettings bool   `yaml:"enable_settings" json:"enable_settings" env:"SECURITY_ENABLE_SETTINGS" env-default:"true"`
	AdminPassword  string `yaml:"admin_password" json:"admin_password" env:"SECURITY_ADMIN_PASSWORD" env-default:""`
	AccessKey      string `yaml:"access_key" json:"access_key" env:"SECURITY_ACCESS_KEY" env-default:""`
}

type ConfigParser struct {
	Host      string         `yaml:"host" json:"host" env:"HOST_RUTOR" env-default:"http://rutor.info"`
	Port      string         `yaml:"port" json:"port" env:"PORT_RUTOR" env-default:"38888"`
	UseProxy  bool           `yaml:"useproxy" json:"useproxy" env:"USEPROXY_RUTOR" env-default:"false"`
	Proxy     string         `yaml:"proxy" json:"proxy" env:"PROXY_RUTOR" env-default:""`
	TmdbToken string         `yaml:"tmdbtoken" json:"tmdbtoken"`
	AigKey    string         `yaml:"aigkey" json:"aigkey"`
	GroqKey   string         `yaml:"groqkey" json:"groqkey" env:"GROQ_KEY" env-default:""`
	GroqModel string         `yaml:"groqmodel" json:"groqmodel" env:"GROQ_MODEL" env-default:"openai/gpt-oss-20b"`
	Releases  ReleasesConfig `yaml:"releases" json:"releases"`
	Security  SecurityConfig `yaml:"security" json:"security"`
}

var (
	cfg ConfigParser
	mu  sync.RWMutex
)

func DefaultConfig() ConfigParser {
	return ConfigParser{
		Host:      "http://rutor.info",
		Port:      "38888",
		GroqModel: "openai/gpt-oss-20b",
		Releases: ReleasesConfig{
			MovieSort:         "release_date",
			MovieMaxAgeMonths: 6,
			TVSort:            "last_air_date",
			TVMaxAgeMonths:    6,
			AnimeSort:         "last_air_date",
			AnimeMaxAgeMonths: 6,
		},
		Security: SecurityConfig{
			EnableSettings: true,
			AdminPassword:  "",
			AccessKey:      "",
		},
	}
}

func LoadConfig() (ConfigParser, error) {
	next := DefaultConfig()
	if _, err := os.Stat(ConfigFile); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return next, err
		}
		if envErr := cleanenv.ReadEnv(&next); envErr != nil {
			return next, envErr
		}
	} else if err := cleanenv.ReadConfig(ConfigFile, &next); err != nil {
		return next, err
	}

	next.Normalize()

	mu.Lock()
	cfg = next
	mu.Unlock()

	return next, nil
}

func SaveConfig(next ConfigParser) (ConfigParser, error) {
	next.Normalize()
	if err := ValidateConfig(next); err != nil {
		return next, err
	}

	buf, err := yaml.Marshal(&next)
	if err != nil {
		return next, err
	}

	if err := os.WriteFile(ConfigFile, buf, 0644); err != nil {
		return next, err
	}

	mu.Lock()
	cfg = next
	ProxyHost = next.Proxy
	UseProxy = next.UseProxy
	mu.Unlock()

	return next, nil
}

func ValidateConfig(next ConfigParser) error {
	next.Normalize()

	if err := validateHTTPURL("host", next.Host); err != nil {
		return err
	}

	port, err := strconv.Atoi(next.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be a number from 1 to 65535")
	}

	if next.Proxy != "" {
		if err := validateProxyURL(next.Proxy); err != nil {
			return err
		}
	}

	return nil
}

func (c *ConfigParser) Normalize() {
	c.Host = strings.TrimSpace(c.Host)
	c.Port = strings.TrimSpace(c.Port)
	c.Proxy = strings.TrimSpace(c.Proxy)
	c.TmdbToken = strings.TrimSpace(c.TmdbToken)
	c.AigKey = strings.TrimSpace(c.AigKey)
	c.GroqKey = strings.TrimSpace(c.GroqKey)
	c.GroqModel = strings.TrimSpace(c.GroqModel)
	if c.GroqModel == "" || c.GroqModel == "llama-3.1-8b-instant" || c.GroqModel == "llama-3.3-70b-versatile" {
		c.GroqModel = "openai/gpt-oss-20b"
	}

	if c.Host == "" {
		c.Host = "http://rutor.info"
	}
	if c.Port == "" {
		c.Port = "38888"
	}

	c.Releases.MovieSort = strings.ToLower(strings.TrimSpace(c.Releases.MovieSort))
	if c.Releases.MovieSort == "" {
		c.Releases.MovieSort = "release_date"
	}
	if c.Releases.MovieSort != "release_date" && c.Releases.MovieSort != "tracker" {
		c.Releases.MovieSort = "release_date"
	}

	c.Releases.TVSort = strings.ToLower(strings.TrimSpace(c.Releases.TVSort))
	if c.Releases.TVSort == "" {
		c.Releases.TVSort = "last_air_date"
	}
	if c.Releases.TVSort != "last_air_date" && c.Releases.TVSort != "tracker" {
		c.Releases.TVSort = "last_air_date"
	}

	c.Releases.AnimeSort = strings.ToLower(strings.TrimSpace(c.Releases.AnimeSort))
	if c.Releases.AnimeSort == "" {
		c.Releases.AnimeSort = "last_air_date"
	}
	if c.Releases.AnimeSort != "last_air_date" && c.Releases.AnimeSort != "tracker" {
		c.Releases.AnimeSort = "last_air_date"
	}

	if c.Releases.MovieMaxAgeMonths < 0 {
		c.Releases.MovieMaxAgeMonths = 0
	}
	if c.Releases.TVMaxAgeMonths < 0 {
		c.Releases.TVMaxAgeMonths = 0
	}
	if c.Releases.AnimeMaxAgeMonths < 0 {
		c.Releases.AnimeMaxAgeMonths = 0
	}

	c.Security.AdminPassword = strings.TrimSpace(c.Security.AdminPassword)
	c.Security.AccessKey = strings.TrimSpace(c.Security.AccessKey)
}

func validateHTTPURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s must be a valid http or https URL", name)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", name)
	}
	return nil
}

func validateProxyURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("proxy must be a valid URL")
	}
	switch u.Scheme {
	case "http", "https", "socks4", "socks4a", "socks5", "socks5h":
		return nil
	default:
		return fmt.Errorf("proxy scheme must be http, https, socks4, socks4a, socks5 or socks5h")
	}
}

func GetConfig() ConfigParser {
	mu.RLock()
	if cfg.Host != "" {
		c := cfg
		mu.RUnlock()
		return c
	}
	mu.RUnlock()

	c, _ := LoadConfig()
	return c
}

func GetReleasesConfig() ReleasesConfig {
	return GetConfig().Releases
}

func GetSecurityConfig() SecurityConfig {
	return GetConfig().Security
}

func RutorHost() string {
	c := GetConfig()
	if c.Host == "" {
		return "http://rutor.info"
	}
	return strings.TrimRight(c.Host, "/")
}

func JoinRutorLink(host, link string) string {
	if link == "" {
		return ""
	}
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		return link
	}
	if !strings.HasPrefix(link, "/") {
		link = "/" + link
	}
	return host + link
}

func ReadConfigParser(vars string) (string, error) {
	c := GetConfig()
	switch vars {
	case "Host":
		return c.Host, nil
	case "Port":
		return c.Port, nil
	case "Proxy":
		return c.Proxy, nil
	case "UseProxy":
		return strconv.FormatBool(c.UseProxy), nil
	case "TmdbToken":
		return c.TmdbToken, nil
	case "AigKey":
		return c.AigKey, nil
	case "GroqKey":
		return c.GroqKey, nil
	case "GroqModel":
		return c.GroqModel, nil
	}
	return "", nil
}
