package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	gazetaURL     = "https://www.gazeta.ru"
	gazetaURLNews = "https://www.gazeta.ru/news/"
)

func GazetaMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск парсера Gazeta.ru...%s\n", ColorYellow, ColorReset)
	_ = getLinksGazeta()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения парсера Gazeta.ru: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksGazeta() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	fmt.Printf("%s[INFO] Начало парсинга ссылок с %s...%s\n", ColorYellow, gazetaURLNews, ColorReset)

	doc, err := GetHTML(gazetaURLNews)
	if err != nil {
		fmt.Printf("%s[ERROR] Не удалось загрузить основную страницу новостей %s после всех попыток. Сбор ссылок прерван.%s\n", ColorRed, gazetaURLNews, ColorReset)
		return getPageGazeta(foundLinks)
	}

	doc.Find("a.b_ear.m_techlisting").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullHref := ""
			if strings.HasPrefix(href, "/") {
				fullHref = gazetaURL + href
			} else if strings.HasPrefix(href, gazetaURL) {
				fullHref = href
			} else if strings.HasPrefix(href, "http") {

			}

			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	})

	if len(foundLinks) > 0 {
		fmt.Printf("%s[INFO] Найдено %d уникальных ссылок на статьи.%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("%s[WARNING] Не найдено ссылок с селектором 'a.b_ear.m_techlisting' на странице %s.%s\n", ColorYellow, gazetaURLNews, ColorReset)
	}

	return getPageGazeta(foundLinks)
}

func getPageGazeta(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Printf("%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorYellow, ColorReset)
		return products
	}
	fmt.Printf("\n%s[INFO] Начало парсинга %d статей с Gazeta.ru...%s\n", ColorYellow, totalLinks, ColorReset)

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
			title = strings.TrimSpace(doc.Find(".headline").First().Text())

			var accumulatedBodyParts []string
			doc.Find(".b_article-text p").Each(func(_ int, pSelection *goquery.Selection) {
				paragraphText := strings.TrimSpace(pSelection.Text())

				if paragraphText != "" &&
					!strings.Contains(paragraphText, "Что думаешь?") &&
					!strings.HasPrefix(paragraphText, "Ранее ") {
					accumulatedBodyParts = append(accumulatedBodyParts, paragraphText)
				}
			})
			body = strings.Join(accumulatedBodyParts, "\n\n")

			if title != "" && body != "" && strings.HasPrefix(body, title) {
				body = strings.TrimPrefix(body, title)
				body = strings.TrimSpace(body)
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
		fmt.Printf("\n%s[INFO] Парсинг статей Gazeta.ru завершен. Собрано %d статей.%s\n", ColorGreen, len(products), ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[ERROR] Парсинг статей Gazeta.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
