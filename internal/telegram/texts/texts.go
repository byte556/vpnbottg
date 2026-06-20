// Package texts — рендерит шаблоны из ru.toml.
//
// Использование:
//
//	txt := texts.T("funnel.welcome.body", map[string]any{
//	    "first_name":   "Игорь",
//	    "trial_price": 1,
//	})
//
// Если ключ не найден — возвращает [missing: key.path] (видно в чате,
// но не валит бот). Если шаблон с битыми {{.field}} — то же.
package texts

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/pelletier/go-toml"
)

//go:embed *.toml
var fs embed.FS

var (
	mu     sync.RWMutex
	tree   map[string]any
	loaded bool
)

func Load() error {
	mu.Lock()
	defer mu.Unlock()
	body, err := fs.ReadFile("ru.toml")
	if err != nil {
		return fmt.Errorf("read ru.toml: %w", err)
	}
	tree = map[string]any{}
	if err := toml.Unmarshal(body, &tree); err != nil {
		return fmt.Errorf("parse ru.toml: %w", err)
	}
	loaded = true
	return nil
}

// T — резолвит "a.b.c" в строку и подставляет args.
func T(path string, args ...map[string]any) string {
	mu.RLock()
	defer mu.RUnlock()
	if !loaded {
		return "[texts not loaded: " + path + "]"
	}

	val, ok := walk(tree, strings.Split(path, "."))
	if !ok {
		return "[missing: " + path + "]"
	}
	str, ok := val.(string)
	if !ok {
		return "[not a string: " + path + "]"
	}

	if len(args) == 0 || len(args[0]) == 0 {
		return str
	}

	tpl, err := template.New(path).Option("missingkey=zero").Parse(str)
	if err != nil {
		return "[template error: " + path + ": " + err.Error() + "]"
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, args[0]); err != nil {
		return "[template execute: " + path + ": " + err.Error() + "]"
	}
	return buf.String()
}

func walk(m map[string]any, parts []string) (any, bool) {
	cur := any(m)
	for _, p := range parts {
		mp, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = mp[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}
