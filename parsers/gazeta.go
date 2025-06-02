package parsers

import (
	"fmt"
	. "parsing_media/utils" // Assuming Data, ColorBlue, ColorYellow, ColorRed, ColorReset, GetHTML, FormatDuration, LimitString are defined here
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

	_ = getLinksGazeta() // Assuming we don't need the result for now

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[GAZETA]%s[INFO] Общее время выполнения парсера Gazeta.ru: %s%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksGazeta() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	doc, err := GetHTML(gazetaURLNews)
	if err != nil {
		fmt.Printf("%s[GAZETA]%s[ERROR] Не удалось загрузить основную страницу новостей %s после всех попыток. Сбор ссылок прерван.%s\n", ColorBlue, ColorRed, gazetaURLNews, ColorReset)
		return getPageGazeta(foundLinks) // Proceed with empty links
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
				// This condition was empty, kept as is.
				// If it was meant to assign fullHref, that logic is missing.
				// For now, it means http links not starting with gazetaURL are ignored.
			}

			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	})

	if len(foundLinks) > 0 {
		// As per Banki template, specific count of found links is not logged here.
		// The warning for no links found is more critical.
		// fmt.Printf("%s[GAZETA]%s[INFO] Найдено %d уникальных ссылок на статьи.%s\n", ColorBlue, ColorYellow, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("%s[GAZETA]%s[WARNING] Не найдено ссылок с селектором 'a.b_ear.m_techlisting' на странице %s.%s\n", ColorBlue, ColorYellow, gazetaURLNews, ColorReset)
	}

	return getPageGazeta(foundLinks)
}

func getPageGazeta(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		// Following Banki.go template, no message here.
		// Warning/error about no links should come from getLinksGazeta.
		return products
	}
	// Removed: fmt.Printf("\n%s[INFO] Начало парсинга %d статей с Gazeta.ru...%s\n", ColorYellow, totalLinks, ColorReset)

	for _, pageURL := range links {
		var title, body string
		//var pageStatusMessage string
		//var statusMessageColor = ColorReset
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			//pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", LimitString(err.Error(), 50))
			//statusMessageColor = ColorRed
			errItems = append(errItems, fmt.Sprintf("%s (ошибка GET: %s)", LimitString(pageURL, 60), err.Error()))
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
				//pageStatusMessage = fmt.Sprintf("Успех: %s", LimitString(title, 60))
				//statusMessageColor = ColorGreen
				parsedSuccessfully = true // Corrected: This should be uncommented to correctly flag successful parses
			} else {
				//statusMessageColor = ColorYellow
				//pageStatusMessage = fmt.Sprintf("Нет данных (T:%t, B:%t): %s", title != "", body != "", LimitString(pageURL, 40))
			}
		}

		if !parsedSuccessfully && err == nil { // Only add to errItems if GetHTML was successful but data extraction failed
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: T:%t, B:%t)", LimitString(pageURL, 60), title != "", body != ""))
		}

		//ProgressBar(title, body, pageStatusMessage, statusMessageColor, i, totalLinks)
	}

	if len(products) > 0 {
		// Removed original success message: fmt.Printf("\n%s[INFO] Парсинг статей Gazeta.ru завершен. Собрано %d статей.%s\n", ColorGreen, len(products), ColorReset)
		// The presence of products implies success. The warning for errItems will cover issues.
		// If all items are processed without error, no specific "success" message is printed, only the total time.
		// This matches Banki.go behavior where a final "gathered X items" isn't explicitly printed unless there are also errors.

		if len(errItems) > 0 {
			fmt.Printf("%s[GAZETA]%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 { // No products collected, but links were attempted
		fmt.Printf("\n%s[GAZETA]%s[ERROR] Парсинг статей Gazeta.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[GAZETA]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	// If totalLinks == 0 initially, no summary message about processing is needed here.

	return products
}
