package core

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"log"
	"strconv"
	"sync"
	"time"
)

const (
	uahCode = 980
	rurCode = 643
)

type CurrencyConverter interface {
	Convert(count float64, srcCode int, dstCode int) float64
	RegisterCallback(callback ConverterCallback)
}

type ConverterCallback func()

type UACbConverter struct {
	m         *sync.Mutex
	client    *resty.Client
	rates     map[int]float64
	callbacks []ConverterCallback
}

type cbrResp struct {
	Valute map[string]currencyRate `json:"Valute"`
}

type currencyRate struct {
	ID       string  `json:"ID"`
	NumCode  string  `json:"NumCode"`
	CharCode string  `json:"CharCode"`
	Nominal  int     `json:"Nominal"`
	Name     string  `json:"Name"`
	Value    float64 `json:"Value"`
	Previous float64 `json:"Previous"`
}

func NewUACbConverter(ctx context.Context, updateInterval time.Duration) (*UACbConverter, error) {
	client := resty.New()
	client.SetTimeout(20 * time.Second)
	client.SetRetryCount(3)
	uaCbConverter := &UACbConverter{
		m:         &sync.Mutex{},
		client:    client,
		rates:     make(map[int]float64),
		callbacks: make([]ConverterCallback, 0),
	}

	go uaCbConverter.loop(ctx, updateInterval)
	if err := uaCbConverter.update(); err != nil {
		return nil, err
	}

	return uaCbConverter, nil
}

func (c *UACbConverter) RegisterCallback(callback ConverterCallback) {
	c.m.Lock()
	defer c.m.Unlock()
	c.callbacks = append(c.callbacks, callback)
}

func (c *UACbConverter) Convert(count float64, srcCode int, dstCode int) float64 {
	c.m.Lock()
	defer c.m.Unlock()

	rur := count * c.rates[srcCode]
	dst := rur / c.rates[dstCode]

	return dst
}

func (c *UACbConverter) update() error {
	resp, err := c.client.
		R().
		Get("https://www.cbr-xml-daily.ru/daily_json.js")
	if err != nil {
		return err
	}

	cbrResp := cbrResp{}
	fmt.Println(string(resp.Body()))
	if err := json.Unmarshal(resp.Body(), &cbrResp); err != nil {
		return err
	}

	c.m.Lock()
	defer c.m.Unlock()
	c.rates = make(map[int]float64)
	for _, rate := range cbrResp.Valute {
		code, _ := strconv.ParseInt(rate.NumCode, 10, 64)
		c.rates[int(code)] = rate.Value / float64(rate.Nominal)
	}
	c.rates[rurCode] = 1.

	for _, cb := range c.callbacks {
		go cb()
	}

	return nil
}

func (c *UACbConverter) loop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.update(); err != nil {
				log.Println("error on currency update")
			} else {
				log.Println("currency updated")
			}
		}
	}
}
