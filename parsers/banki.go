package parsers

import (
	"encoding/json"
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type NewsItemJSONBanki struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type ListViewItemsJSONBanki map[string][]NewsItemJSONBanki

type ModuleOptionsJSONBanki struct {
	ListViewItems ListViewItemsJSONBanki `json:"listViewItems"`
	PageRoute     string                 `json:"pageRoute"`
}

const (
	bankiURL     = "https://www.banki.ru"
	bankiURLNews = "https://www.banki.ru/news/lenta/"
)

func BankiMain() {
	totalStartTime := time.Now()

	_ = getLinksBanki()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[BANKI]%s[INFO] Парсер Banki.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksBanki() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	doc, err := GetHTML(bankiURLNews)
	if err != nil {
		fmt.Printf("%s[BANKI]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, bankiURLNews, err, ColorReset)
		return getPageBanki(foundLinks)
	}

	var jsonData string
	doc.Find("div[data-module*='NewsBundle/app/desktop/lenta-list']").EachWithBreak(func(i int, s *goquery.Selection) bool {
		optionsStr, exists := s.Attr("data-module-options")
		if exists {
			jsonData = optionsStr
			return false
		}
		return true
	})

	if jsonData == "" {
		fmt.Printf("%s[BANKI]%s[WARNING] Не найден JSON с данными о новостях (атрибут 'data-module-options') на странице %s.%s\n", ColorBlue, ColorYellow, bankiURLNews, ColorReset)
		return getPageBanki(foundLinks)
	}

	var moduleOpts ModuleOptionsJSONBanki
	err = json.Unmarshal([]byte(jsonData), &moduleOpts)
	if err != nil {
		fmt.Printf("%s[BANKI]%s[ERROR] Не удалось распарсить JSON, извлеченный со страницы %s: %v%s\n", ColorBlue, ColorRed, bankiURLNews, err, ColorReset)
		return getPageBanki(foundLinks)
	}

	if len(moduleOpts.ListViewItems) == 0 {
		fmt.Printf("%s[BANKI]%s[INFO] В извлеченном JSON поле listViewItems пусто. Новостей не найдено на %s.%s\n", ColorBlue, ColorYellow, bankiURLNews, ColorReset)
		return getPageBanki(foundLinks)
	}

	pageRoute := moduleOpts.PageRoute
	if pageRoute == "" {
		pageRoute = "/news/lenta/"
	}
	if !strings.HasPrefix(pageRoute, "/") {
		pageRoute = "/" + pageRoute
	}
	pageRoute = strings.TrimSuffix(pageRoute, "/")

	for _, newsItemsOnDate := range moduleOpts.ListViewItems {
		for _, item := range newsItemsOnDate {
			fullHref := fmt.Sprintf("%s%s?id=%d", bankiURL, pageRoute, item.ID)
			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	}

	if len(foundLinks) <= 0 {
		fmt.Printf("%s[BANKI]%s[WARNING] Не найдено ссылок для парсинга из JSON на странице %s.%s\n", ColorBlue, ColorYellow, bankiURLNews, ColorReset)
	}

	return getPageBanki(foundLinks)
}

func getPageBanki(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)
	locationPlus3 := time.FixedZone("UTC+3", 3*60*60)
	dateTimeStr := "02.01.2006 15:04"

	if totalLinks == 0 {
		return products
	}

	for _, pageURL := range links {
		var title, body string
		var parsDate time.Time
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("(ошибка GET: %s)", err.Error()))
		} else {
			title = strings.TrimSpace(doc.Find("h1[class*='text-header-0']").First().Text())

			var bodyBuilder strings.Builder
			doc.Find("div.l6d291019").Find("p, a, span, ol, li").Each(func(_ int, s *goquery.Selection) {
				partText := strings.TrimSpace(s.Text())
				if strings.Contains(partText, "Актуальные котировки, аналитические обзоры") ||
					strings.HasPrefix(partText, "Самый большой финансовый маркетплейс в России") ||
					strings.Contains(partText, "Оставайтесь в курсе событий") ||
					strings.Contains(partText, "Топ 3 дебетовых карт") {
					return
				}
				if partText != "" {
					if bodyBuilder.Len() > 0 {
						bodyBuilder.WriteString("\n\n")
					}
					bodyBuilder.WriteString(partText)
				}
			})
			body = bodyBuilder.String()

			dateTextRaw := doc.Find("span[class*='l51e0a7a5']").First().Text()
			dateTextClean := strings.TrimSpace(dateTextRaw)

			dateToParse := dateTextClean

			if strings.HasPrefix(dateTextClean, "Дата публикации: ") {
				dateToParse = strings.TrimSpace(strings.TrimPrefix(dateTextClean, "Дата публикации: "))
			}

			if dateToParse != "" {
				parsDate, _ = time.ParseInLocation(dateTimeStr, dateToParse, locationPlus3)
			}

			if title != "" && body != "" && !parsDate.IsZero() {
				products = append(products, Data{Href: pageURL, Title: title, Body: body, Date: parsDate})
				parsedSuccessfully = true
			}
		}

		if !parsedSuccessfully && err == nil {
			var reasons []string
			if title == "" {
				reasons = append(reasons, "T:false")
			}
			if body == "" {
				reasons = append(reasons, "B:false")
			}
			if parsDate.IsZero() {
				reasons = append(reasons, "D:false")
			}
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", pageURL, strings.Join(reasons, ", ")))
		}
	}

	if len(products) > 0 {
		if len(errItems) > 0 {
			fmt.Printf("%s[BANKI]%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[BANKI]%s[ERROR] Парсинг статей Banki.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[BANKI]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
