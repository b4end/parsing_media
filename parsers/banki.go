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

	fmt.Printf("%s[INFO] Запуск парсера Banki.ru...%s\n", ColorYellow, ColorReset)
	_ = getLinksBanki()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения парсера Banki.ru: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksBanki() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	fmt.Printf("%s[INFO] Начало сбора ссылок с %s (извлечение JSON)...%s\n", ColorYellow, bankiURLNews, ColorReset)

	doc, err := GetHTML(bankiURLNews)
	if err != nil {
		fmt.Printf("%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorRed, bankiURLNews, err, ColorReset)
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
		fmt.Printf("%s[WARNING] Не найден JSON с данными о новостях (атрибут 'data-module-options') на странице %s.%s\n", ColorYellow, bankiURLNews, ColorReset)
		return getPageBanki(foundLinks)
	}

	var moduleOpts ModuleOptionsJSONBanki
	err = json.Unmarshal([]byte(jsonData), &moduleOpts)
	if err != nil {
		fmt.Printf("%s[ERROR] Не удалось распарсить JSON, извлеченный со страницы %s: %v%s\n", ColorRed, bankiURLNews, err, ColorReset)
		return getPageBanki(foundLinks)
	}

	if len(moduleOpts.ListViewItems) == 0 {
		fmt.Printf("%s[INFO] В извлеченном JSON поле listViewItems пусто. Новостей не найдено на %s.%s\n", ColorYellow, bankiURLNews, ColorReset)
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

	if len(foundLinks) > 0 {
		fmt.Printf("%s[INFO] Собрано %d уникальных ссылок на статьи (из JSON на странице).%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("%s[WARNING] Не найдено ссылок для парсинга из JSON на странице %s.%s\n", ColorYellow, bankiURLNews, ColorReset)
	}

	return getPageBanki(foundLinks)
}

func getPageBanki(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Printf("%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorYellow, ColorReset)
		return products
	}
	fmt.Printf("\n%s[INFO] Начало парсинга %d статей с Banki.ru...%s\n", ColorYellow, totalLinks, ColorReset)

	for i, pageURL := range links {
		var title, body string
		var pageStatusMessage string
		var statusMessageColor = ColorReset
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", LimitString(err.Error(), 50))
			statusMessageColor = ColorRed
			errItems = append(errItems, fmt.Sprintf("%s (ошибка GET: %s)", LimitString(pageURL, 60), LimitString(err.Error(), 50)))
		} else {
			title = strings.TrimSpace(doc.Find("h1[class*='text-header-0']").First().Text())

			var bodyBuilder strings.Builder
			doc.Find("div.l6d291019").Find("p, a, span, ol, li").Each(func(_ int, s *goquery.Selection) {

				partText := strings.TrimSpace(s.Text())
				if strings.Contains(partText, "Актуальные котировки, аналитические обзоры") ||
					strings.HasPrefix(partText, "Самый большой финансовый маркетплейс в России") ||
					strings.Contains(partText, "Оставайтесь в курсе событий") {
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

			if title != "" && body != "" {
				products = append(products, Data{Title: title, Body: body})
				pageStatusMessage = fmt.Sprintf("Успех: %s", LimitString(title, 60))
				statusMessageColor = ColorGreen
				parsedSuccessfully = true
			} else {
				statusMessageColor = ColorYellow
				pageStatusMessage = fmt.Sprintf("Нет данных (T:%t, B:%t): %s", title != "", body != "", LimitString(pageURL, 40))
			}
		}

		if !parsedSuccessfully && err == nil {
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: T:%t, B:%t)", LimitString(pageURL, 60), title != "", body != ""))
		}

		ProgressBar(title, body, pageStatusMessage, statusMessageColor, i, totalLinks)
	}

	if len(products) > 0 {
		fmt.Printf("\n%s[INFO] Парсинг статей Banki.ru завершен. Собрано %d статей.%s\n", ColorGreen, len(products), ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[ERROR] Парсинг статей Banki.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
