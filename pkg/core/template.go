package core

import (
	"encoding/json"
	"os"
	"regexp"
	"sync"
)

var priceRe = regexp.MustCompile(`\$price:(\d+(\.\d+)?)`)

type TemplateManager struct {
	path      string
	templates map[int64]map[int64]TemplateText
	m         *sync.Mutex
}

type TemplateText struct {
	Text      string `json:"Text"`
	IsCaption bool   `json:"IsCaption"`
}

type Template struct {
	ChatID       int64
	MessageID    int64
	TemplateText TemplateText
}

func NewTemplateManager(configPath string) (*TemplateManager, error) {
	manager := &TemplateManager{
		path:      configPath,
		templates: make(map[int64]map[int64]TemplateText),
		m:         &sync.Mutex{},
	}

	if _, err := os.Stat(configPath); err != nil {
		if err := manager.dump(); err != nil {
			return nil, err
		}
	}
	if err := manager.load(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *TemplateManager) ListTemplates() []Template {
	m.m.Lock()
	defer m.m.Unlock()
	result := make([]Template, 0)
	for chatID, messages := range m.templates {
		for messaheID, template := range messages {
			result = append(result, Template{
				ChatID:       chatID,
				MessageID:    messaheID,
				TemplateText: template,
			})
		}
	}
	return result
}

func (m *TemplateManager) AddTemplate(chatID int64, messageID int64, template TemplateText) error {
	m.m.Lock()
	defer m.m.Unlock()
	watchedMessages, ok := m.templates[chatID]
	if !ok {
		watchedMessages = make(map[int64]TemplateText)
		m.templates[chatID] = watchedMessages
	}

	watchedMessages[messageID] = template
	if err := m.dump(); err != nil {
		return err
	}

	return nil
}

func (m *TemplateManager) DeleteTemplate(chatID int64, messageID int64) error {
	m.m.Lock()
	defer m.m.Unlock()
	if watchedTemplates, ok := m.templates[chatID]; ok {
		delete(watchedTemplates, messageID)
	}
	if err := m.dump(); err != nil {
		return err
	}
	return nil
}

func (m *TemplateManager) dump() error {
	data, err := json.Marshal(m.templates)
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.path, data, 0644); err != nil {
		return err
	}
	return nil
}

func (m *TemplateManager) load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &m.templates)
}

func IsTemplate(text string) bool {
	return priceRe.FindAllString(text, -1) != nil
}
