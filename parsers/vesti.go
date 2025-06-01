package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	vestiURL     = "https://www.vesti.ru"
	vestiURLNews = "https://www.vesti.ru/news"
)

func VestiMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск парсера Vesti.ru...%s\n", ColorYellow, ColorReset)
	_ = getLinksVesti()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения парсера Vesti.ru: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksVesti() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	fmt.Printf("%s[INFO] Начало парсинга ссылок с %s...%s\n", ColorYellow, vestiURLNews, ColorReset)

	doc, err := GetHTML(vestiURLNews)
	if err != nil {
		fmt.Printf("%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorRed, vestiURLNews, err, ColorReset)
		return getPageVesti(foundLinks)
	}

	doc.Find("a.list__pic-wrapper").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullHref := ""
			if strings.HasPrefix(href, "/") {
				fullHref = vestiURL + href
			} else if strings.HasPrefix(href, vestiURL) {
				fullHref = href
			}

			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	})

	if len(foundLinks) > 0 {
		fmt.Printf("%s[INFO] Найдено %d уникальных ссылок на статьи (с HTML страницы).%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("%s[WARNING] Не найдено ссылок с селектором 'a.list__pic-wrapper' на странице %s.%s\n", ColorYellow, vestiURLNews, ColorReset)
	}

	return getPageVesti(foundLinks)
}

func getPageVesti(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Printf("%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorYellow, ColorReset)
		return products
	}
	fmt.Printf("\n%s[INFO] Начало парсинга %d статей с Vesti.ru...%s\n", ColorYellow, totalLinks, ColorReset)

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
			title = strings.TrimSpace(doc.Find("h1.article__title").First().Text())
			var bodyBuilder strings.Builder
			doc.Find(".js-mediator-article").Find("p, blockquote").Each(func(_ int, s *goquery.Selection) {
				partText := strings.TrimSpace(s.Text())
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
		fmt.Printf("\n%s[INFO] Парсинг статей Vesti.ru завершен. Собрано %d статей.%s\n", ColorGreen, len(products), ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[ERROR] Парсинг статей Vesti.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
