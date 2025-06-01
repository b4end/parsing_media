package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	lentaURL     = "https://lenta.ru"
	lentaURLPage = "https://lenta.ru/parts/news/"
)

func LentaMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск парсера Lenta.ru...%s\n", ColorYellow, ColorReset)
	_ = getLinksLenta()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения парсера Lenta.ru: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksLenta() []Data {
	var foundLinks []string

	fmt.Printf("%s[INFO] Начало парсинга ссылок с %s...%s\n", ColorYellow, lentaURLPage, ColorReset)

	doc, err := GetHTML(lentaURLPage)
	if err != nil {
		fmt.Printf("%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorRed, lentaURLPage, err, ColorReset)
		return getPageLenta(foundLinks)
	}

	doc.Find("a.card-full-news._parts-news").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if !strings.HasPrefix(href, "https://") && strings.HasPrefix(href, "/") {
				linkToAdd := lentaURL + href
				isDuplicate := false
				for _, l := range foundLinks {
					if l == linkToAdd {
						isDuplicate = true
						break
					}
				}
				if !isDuplicate {
					foundLinks = append(foundLinks, linkToAdd)
				}
			}
		}
	})

	if len(foundLinks) > 0 {
		fmt.Printf("%s[INFO] Найдено %d ссылок на статьи.%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("%s[WARNING] Не найдено ссылок с селектором 'a.card-full-news._parts-news' на странице %s.%s\n", ColorYellow, lentaURLPage, ColorReset)
	}

	return getPageLenta(foundLinks)
}

func getPageLenta(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Printf("%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorYellow, ColorReset)
		return products
	}
	fmt.Printf("\n%s[INFO] Начало парсинга %d статей с Lenta.ru...%s\n", ColorYellow, totalLinks, ColorReset)

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
			title = strings.TrimSpace(doc.Find(".topic-body__title").First().Text())

			doc.Find(".topic-body__content-text").Each(func(i int, s *goquery.Selection) {
				currentTextPart := strings.TrimSpace(s.Text())
				if currentTextPart != "" {
					if body != "" {
						body += "\n\n"
					}
					body += currentTextPart
				}
			})

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
		fmt.Printf("\n%s[INFO] Парсинг статей Lenta.ru завершен. Собрано %d статей.%s\n", ColorGreen, len(products), ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[ERROR] Парсинг статей Lenta.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
