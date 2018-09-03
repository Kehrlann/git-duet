package duet

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v2"
)

// Pairs wraps the git authors file with logic for looking up pairs based on initials
// and building email addresses
type Pairs struct {
	file        *pairsFile
	emailLookup string
}

// Pair represents a single pair
type Pair struct {
	Name     string
	Email    string
	Initials string
	Username string
}

type pairsFile struct {
	Pairs          map[string]string `yaml:"authors"`
	Email          emailConfig       `yaml:"email"`
	EmailAddresses map[string]string `yaml:"email_addresses"`
	EmailTemplate  string            `yaml:"email_template"`
}

type emailConfig struct {
	Prefix string
	Domain string
}

var pairsKey = regexp.MustCompile(`(?m)^pairs:`)

// NewPairsFromFile parses the given yml authors file (see README.md for file structure)
// Uses emailLookup as external command to determine pair email address if set
func NewPairsFromFile(filename string, emailLookup string) (a *Pairs, err error) {
	af := &pairsFile{}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	contents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	// Hack to also support `pairs:` as the key
	contents = pairsKey.ReplaceAll(contents, []byte("authors:"))

	err = yaml.Unmarshal(contents, &af)
	if err != nil {
		return nil, fmt.Errorf("could not parse %s: %+v", filename, err)
	}

	return &Pairs{
		file:        af,
		emailLookup: emailLookup,
	}, nil
}

var templateFuncs = template.FuncMap{
	"toLower": strings.ToLower,
	"toUpper": strings.ToUpper,
	"split":   strings.Split,
	"replace": strings.Replace,
}

func (a *Pairs) buildEmail(initials, name, username string) (email string, err error) {
	if a.emailLookup != "" {
		var out bytes.Buffer

		cmd := exec.Command(a.emailLookup, initials, name, username)
		cmd.Stdout = &out

		if err := cmd.Run(); err != nil {
			return "", err
		}

		email = strings.TrimSpace(out.String())
		if email != "" {
			return email, nil
		}
	}

	if e, ok := a.file.EmailAddresses[initials]; ok {
		email = e
	} else if a.file.EmailTemplate != "" {
		var out bytes.Buffer

		t, err := template.New("email").Funcs(templateFuncs).Parse(a.file.EmailTemplate)
		if err != nil {
			return "", err
		}

		if err = t.Execute(&out, Pair{Initials: initials, Name: name, Username: username}); err != nil {
			return "", err
		}
		email = out.String()

	} else if username != "" {
		email = fmt.Sprintf("%s@%s", strings.TrimSpace(username), a.file.Email.Domain)
	} else {
		names := strings.SplitN(name, " ", 2)
		if len(names) == 2 {
			email = fmt.Sprintf(
				"%c.%s@%s",
				strings.ToLower(strings.TrimSpace(names[0]))[0],
				strings.ToLower(strings.TrimSpace(names[1])),
				a.file.Email.Domain)
		} else {
			email = fmt.Sprintf("%s@%s", strings.ToLower(strings.TrimSpace(names[0])), a.file.Email.Domain)
		}
	}

	return email, nil
}

// ByInitials returns the pair with the given initials
// The email is determined from the first non-empty value during the following steps:
// - Run external lookup if provided during initialization
// - Pull from `email_addresses` map in config
// - Build using `email_template` if provided
// - Build using username (if provided) and domain
// - If two names, build using first initial followed by . followed by last name and domain
// - If one name, build using name followed by domain
func (a *Pairs) ByInitials(initials string) (pair *Pair, err error) {
	pairString, ok := a.file.Pairs[initials]
	if !ok {
		return nil, fmt.Errorf("unknown initials %s", initials)
	}

	pairParts := strings.SplitN(pairString, ";", 2)
	name := strings.TrimSpace(pairParts[0])
	username := ""
	if len(pairParts) == 2 {
		username = strings.TrimSpace(pairParts[1])
	}

	email, err := a.buildEmail(initials, name, username)
	if err != nil {
		return nil, err
	}

	return &Pair{
		Name:     name,
		Email:    email,
		Username: username,
		Initials: initials,
	}, nil
}
