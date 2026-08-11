package seed

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

// Faker generates realistic fake data for testing and database seeding without external dependencies.
type Faker struct {
	mu sync.Mutex
}

// NewFaker creates a new Faker instance.
func NewFaker() *Faker {
	return &Faker{}
}

var DefaultFaker = NewFaker()

var (
	firstNames = []string{"Alex", "Jordan", "Taylor", "Morgan", "Sam", "Chris", "Pat", "Riley", "Dakota", "Avery", "Cameron", "Devon", "Harper", "Quinn", "Rowan", "Skyler"}
	lastNames  = []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas"}
	domains    = []string{"gmail.com", "yahoo.com", "outlook.com", "example.com", "company.io", "tech.dev", "domain.org"}
	companies  = []string{"Acme Corp", "Apex Technologies", "Starlight Systems", "Nexus Dynamic", "CloudScale", "Vortex Labs", "Hyperion Corp", "Quantum Soft"}
	streets    = []string{"Market St", "Main St", "Broadway", "Fifth Ave", "Pine St", "Oak Ave", "Cedar Rd", "Elm St", "Park Ave", "Washington St"}
	cities     = []string{"San Francisco", "New York", "Austin", "Seattle", "Chicago", "Boston", "Denver", "Los Angeles", "Miami", "Atlanta"}
	words      = []string{"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit", "sed", "do", "eiusmod", "tempor", "incididunt", "ut", "labore", "et", "dolore", "magna", "aliqua"}
)

func (f *Faker) randInt(max int) int {
	if max <= 0 {
		return 0
	}
	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(nBig.Int64())
}

// Name generates a random full name (e.g. "Alex Smith").
func (f *Faker) Name() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	firstName := firstNames[f.randInt(len(firstNames))]
	lastName := lastNames[f.randInt(len(lastNames))]
	return firstName + " " + lastName
}

// Email generates a random email address (e.g. "alex.smith42@gmail.com").
func (f *Faker) Email() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	firstName := strings.ToLower(firstNames[f.randInt(len(firstNames))])
	lastName := strings.ToLower(lastNames[f.randInt(len(lastNames))])
	domain := domains[f.randInt(len(domains))]
	num := f.randInt(999) + 1
	return fmt.Sprintf("%s.%s%d@%s", firstName, lastName, num, domain)
}

// Password generates a random secure password string.
func (f *Faker) Password() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + "!Aa1"
}

// UUID generates a random UUID v4 string.
func (f *Faker) UUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Phone generates a random US-formatted phone number (e.g. "+1-555-019-2834").
func (f *Faker) Phone() string {
	return fmt.Sprintf("+1-555-%03d-%04d", f.randInt(900)+100, f.randInt(9000)+1000)
}

// Address generates a random street address (e.g. "742 Evergreen Terrace, San Francisco, CA").
func (f *Faker) Address() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	num := f.randInt(9900) + 100
	street := streets[f.randInt(len(streets))]
	city := cities[f.randInt(len(cities))]
	return fmt.Sprintf("%d %s, %s", num, street, city)
}

// Company generates a random company name.
func (f *Faker) Company() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return companies[f.randInt(len(companies))]
}

// Sentence generates a random dummy text sentence.
func (f *Faker) Sentence() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := f.randInt(5) + 5
	var sb []string
	for i := 0; i < count; i++ {
		sb = append(sb, words[f.randInt(len(words))])
	}
	res := strings.Join(sb, " ")
	return strings.ToUpper(res[:1]) + res[1:] + "."
}

// Paragraph generates a paragraph of text sentences.
func (f *Faker) Paragraph() string {
	var sentences []string
	for i := 0; i < 3; i++ {
		sentences = append(sentences, f.Sentence())
	}
	return strings.Join(sentences, " ")
}

// Select picks a random option from the provided options slice.
func (f *Faker) Select(options ...string) string {
	if len(options) == 0 {
		return ""
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return options[f.randInt(len(options))]
}

// IntRange returns a random integer between min and max inclusive.
func (f *Faker) IntRange(min, max int) int {
	if min >= max {
		return min
	}
	return min + f.randInt(max-min+1)
}

// FloatRange returns a random float64 between min and max.
func (f *Faker) FloatRange(min, max float64) float64 {
	if min >= max {
		return min
	}
	r := float64(f.randInt(100000)) / 100000.0
	return min + r*(max-min)
}

// Bool returns a random boolean value.
func (f *Faker) Bool() bool {
	return f.randInt(2) == 1
}

// PastDate returns a time.Time in the past up to 365 days ago.
func (f *Faker) PastDate() time.Time {
	days := f.randInt(365) + 1
	return time.Now().AddDate(0, 0, -days)
}

// FutureDate returns a time.Time in the future up to 365 days ahead.
func (f *Faker) FutureDate() time.Time {
	days := f.randInt(365) + 1
	return time.Now().AddDate(0, 0, days)
}
