package core

import (
	"context"
	"errors"
	"log"
	"math"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	ctx     context.Context
	botAPI  *tgbotapi.BotAPI
	cancel  context.CancelFunc
	updates tgbotapi.UpdatesChannel
	done    chan interface{}
	m       sync.Mutex
	started bool
	stopped bool

	renderer        *Renderer
	templateManager *TemplateManager
	converter       CurrencyConverter
}

func NewBot(botAPI *tgbotapi.BotAPI, templateManager *TemplateManager, renderer *Renderer, converter CurrencyConverter) *Bot {
	bot := &Bot{
		ctx:             nil,
		botAPI:          botAPI,
		cancel:          nil,
		updates:         nil,
		done:            make(chan interface{}),
		m:               sync.Mutex{},
		started:         false,
		stopped:         false,
		templateManager: templateManager,
		renderer:        renderer,
		converter:       converter,
	}

	return bot

}

func (b *Bot) Start(ctx context.Context) error {
	b.m.Lock()
	defer b.m.Unlock()

	if b.started {
		return errors.New("started already")
	}

	b.ctx, b.cancel = context.WithCancel(ctx)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.botAPI.GetUpdatesChan(u)
	go b.loop(updates)

	b.converter.RegisterCallback(b.updateAllMessages)
	go b.updateAllMessages()

	b.started = true

	return nil
}

func (b *Bot) Stop(timeout time.Duration) error {
	b.m.Lock()
	defer b.m.Unlock()

	if !b.started || b.stopped {
		return errors.New("invalid state")
	}

	b.cancel()
	select {
	case <-b.done:
		return nil
	case <-time.After(timeout):
		return errors.New("can't close bot due to timeout")
	}
}

func (b *Bot) loop(updates tgbotapi.UpdatesChannel) {
	log.Println("starting bot loop")
	for {
		select {
		case <-b.ctx.Done():
			b.done <- true
			return
		case update := <-updates:
			go b.processUpdate(update)
		}
	}
}

func (b *Bot) processUpdate(update tgbotapi.Update) {
	log.Println("got update:", update)

	var msg *tgbotapi.Message
	if update.ChannelPost != nil {
		msg = update.ChannelPost
	} else if update.EditedChannelPost != nil {
		msg = update.EditedChannelPost
	} else {
		log.Println("nothing interesting, skip it")
		return
	}

	text := TemplateText{
		Text:      msg.Text,
		IsCaption: false,
	}
	entities := msg.Entities

	if text.Text == "" {
		text = TemplateText{
			Text:      msg.Caption,
			IsCaption: true,
		}
		entities = msg.CaptionEntities
	}

	if !IsTemplate(text.Text) {
		log.Println("not a template, skip it")
		if err := b.templateManager.DeleteTemplate(msg.Chat.ID, int64(msg.MessageID)); err != nil {
			log.Println("Error on removing template:", err.Error())
		}
		return
	}

	text.Text = format(text.Text, entities)
	log.Println("got template", text)
	if err := b.templateManager.AddTemplate(msg.Chat.ID, int64(msg.MessageID), text); err != nil {
		log.Println("Error on adding template:", err.Error())
		return
	}
	log.Println("template added, update it")

	if err := b.updateMessage(msg.Chat.ID, msg.MessageID, text); err != nil {
		log.Println("Failed to update message:", err.Error())
		return
	}
}

func format(originalText string, entities []tgbotapi.MessageEntity) string {
	if len(entities) == 0 {
		return originalText
	}
	text := []rune(originalText)

	formatedText := ""
	endIndex := 0
	casualTag := func(tag string) func(string) string {
		return func(s string) string {
			return "<" + tag + ">" + s + "</" + tag + ">"
		}
	}
	for _, entity := range entities {
		if endIndex > entity.Offset || endIndex >= len(text) {
			continue
		}
		tag := func(s string) string { return s }
		if entity.IsBold() {
			tag = casualTag("b")
		}
		if entity.IsItalic() {
			tag = casualTag("i")
		}
		if entity.IsTextLink() {
			tag = func(ref string) func(s string) string {
				return func(s string) string {
					return "<a href=\"" + ref + "\">" + s + "</a>"
				}
			}(entity.URL)
		}
		prefix := string(text[endIndex:entity.Offset])
		body := string(text[entity.Offset:int(math.Min(float64(entity.Offset+entity.Length), float64(len(text))))])
		formatedText += prefix + tag(body)
		endIndex = entity.Offset + entity.Length
	}
	if endIndex < len(text) {
		formatedText += string(text[endIndex:])
	}
	return formatedText
}

func (b *Bot) updateMessage(chatID int64, messageID int, text TemplateText) error {
	b.m.Lock()
	defer b.m.Unlock()
	rendered := b.renderer.Render(text.Text)
	var msg tgbotapi.Chattable
	if text.IsCaption {
		msg = &tgbotapi.EditMessageCaptionConfig{
			BaseEdit: tgbotapi.BaseEdit{
				ChatID:    chatID,
				MessageID: messageID,
			},
			ParseMode: tgbotapi.ModeHTML,
			Caption:   rendered,
		}
	} else {
		msg = &tgbotapi.EditMessageTextConfig{
			BaseEdit: tgbotapi.BaseEdit{
				ChatID:    chatID,
				MessageID: messageID,
			},
			ParseMode: tgbotapi.ModeHTML,
			Text:      rendered,
		}
	}
	if _, err := b.botAPI.Send(msg); err != nil {
		return err
	}

	return nil
}

func (b *Bot) updateAllMessages() {
	for _, template := range b.templateManager.ListTemplates() {
		if err := b.updateMessage(template.ChatID, int(template.MessageID), template.TemplateText); err != nil {
			log.Println("error on updating template", err.Error())
		}
	}
}
