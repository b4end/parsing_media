package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	riaNewsPageURL = "https://ria.ru/lenta/"
)

func RiaMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск парсера RIA.ru...%s\n", ColorYellow, ColorReset)
	_ = getLinksRia()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения парсера RIA.ru: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksRia() []Data {
	var foundLinks []string

	fmt.Printf("%s[INFO] Начало парсинга ссылок с %s...%s\n", ColorYellow, riaNewsPageURL, ColorReset)

	doc, err := GetHTML(riaNewsPageURL)
	if err != nil {
		fmt.Printf("%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorRed, riaNewsPageURL, err, ColorReset)
		return getPageRia(foundLinks)
	}

	doc.Find("a.list-item__title.color-font-hover-only").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if strings.HasPrefix(href, "https://ria.ru") {
				isDuplicate := false
				for _, l := range foundLinks {
					if l == href {
						isDuplicate = true
						break
					}
				}
				if !isDuplicate {
					foundLinks = append(foundLinks, href)
				}
			}
		}
	})

	if len(foundLinks) > 0 {
		fmt.Printf("%s[INFO] Найдено %d ссылок на статьи.%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("%s[WARNING] Не найдено ссылок с селектором 'a.list-item__title.color-font-hover-only' на странице %s.%s\n", ColorYellow, riaNewsPageURL, ColorReset)
	}

	return getPageRia(foundLinks)
}

func getPageRia(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Printf("%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorYellow, ColorReset)
		return products
	}
	fmt.Printf("\n%s[INFO] Начало парсинга %d статей с RIA.ru...%s\n", ColorYellow, totalLinks, ColorReset)

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
			title = strings.TrimSpace(doc.Find(".article__title").First().Text())

			articleBodyNode := doc.Find(".article__body")
			if articleBodyNode.Length() > 0 {
				articleBodyNode.Find(".article__text, .article__quote-text").Each(func(j int, s *goquery.Selection) {
					currentTextPart := strings.TrimSpace(s.Text())
					if currentTextPart != "" {
						if body != "" {
							body += "\n\n"
						}
						body += currentTextPart
					}
				})
			} else {
				doc.Find(".article__text, .article__quote-text").Each(func(j int, s *goquery.Selection) {
					currentTextPart := strings.TrimSpace(s.Text())
					if currentTextPart != "" {
						if body != "" {
							body += "\n\n"
						}
						body += currentTextPart
					}
				})
			}

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
		fmt.Printf("\n%s[INFO] Парсинг статей RIA.ru завершен. Собрано %d статей.%s\n", ColorGreen, len(products), ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[ERROR] Парсинг статей RIA.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
