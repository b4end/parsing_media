package parsers

import (
	"fmt"
	. "parsing_media/utils" // Assuming Data, ColorBlue, ColorYellow, ColorRed, ColorReset, GetHTML, FormatDuration, LimitString are defined here
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	riaNewsPageURL = "https://ria.ru/lenta/"
	// riaBaseURL = "https://ria.ru" // Not strictly needed if hrefs are absolute, but good practice if relative links were possible
)

func RiaMain() {
	totalStartTime := time.Now()

	// Removed: fmt.Printf("%s[INFO] Запуск парсера RIA.ru...%s\n", ColorYellow, ColorReset)
	_ = getLinksRia() // Assuming we don't need the result for now

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[RIA]%s[INFO] Общее время выполнения парсера RIA.ru: %s%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksRia() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool) // Added for efficient duplicate checking

	// Removed: fmt.Printf("%s[INFO] Начало парсинга ссылок с %s...%s\n", ColorYellow, riaNewsPageURL, ColorReset)

	doc, err := GetHTML(riaNewsPageURL)
	if err != nil {
		fmt.Printf("%s[RIA]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, riaNewsPageURL, err, ColorReset)
		return getPageRia(foundLinks) // Proceed with empty links
	}

	doc.Find("a.list-item__title.color-font-hover-only").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			// Assuming href is already an absolute URL starting with "https://ria.ru" as per original logic
			if strings.HasPrefix(href, "https://ria.ru") {
				if !seenLinks[href] {
					seenLinks[href] = true
					foundLinks = append(foundLinks, href)
				}
			}
			// If href could be relative, we'd need logic like:
			// fullHref := ""
			// if strings.HasPrefix(href, "/") {
			// 	fullHref = riaBaseURL + href
			// } else if strings.HasPrefix(href, "https://ria.ru") {
			//   fullHref = href
			// }
			// if fullHref != "" && !seenLinks[fullHref] { ... }
		}
	})

	if len(foundLinks) > 0 {
		// Removed: fmt.Printf("%s[INFO] Найдено %d ссылок на статьи.%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("%s[RIA]%s[WARNING] Не найдено ссылок с селектором 'a.list-item__title.color-font-hover-only' на странице %s.%s\n", ColorBlue, ColorYellow, riaNewsPageURL, ColorReset)
	}

	return getPageRia(foundLinks)
}

func getPageRia(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		// Removed: fmt.Printf("%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorYellow, ColorReset)
		return products
	}
	// Removed: fmt.Printf("\n%s[INFO] Начало парсинга %d статей с RIA.ru...%s\n", ColorYellow, totalLinks, ColorReset)

	for _, pageURL := range links {
		var title, body string
		//var pageStatusMessage string
		//var statusMessageColor = ColorReset
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			//pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", LimitString(err.Error(), 50))
			//statusMessageColor = ColorRed
			errItems = append(errItems, fmt.Sprintf("%s (ошибка GET: %s)", LimitString(pageURL, 60), LimitString(err.Error(), 50)))
		} else {
			title = strings.TrimSpace(doc.Find(".article__title").First().Text())

			var bodyBuilder strings.Builder // Use strings.Builder for efficient concatenation
			// body = "" // Not strictly needed if bodyBuilder is used correctly and reset implicitly by being scoped here

			var targetNodes *goquery.Selection
			articleBodyNode := doc.Find(".article__body")
			if articleBodyNode.Length() > 0 {
				targetNodes = articleBodyNode.Find(".article__text, .article__quote-text")
			} else {
				// Fallback if .article__body is not present, search within the whole document.
				// This matches the original logic's intent.
				targetNodes = doc.Find(".article__text, .article__quote-text")
			}

			targetNodes.Each(func(j int, s *goquery.Selection) {
				currentTextPart := strings.TrimSpace(s.Text())
				if currentTextPart != "" {
					if bodyBuilder.Len() > 0 {
						bodyBuilder.WriteString("\n\n")
					}
					bodyBuilder.WriteString(currentTextPart)
				}
			})
			body = bodyBuilder.String()

			if title != "" && body != "" {
				products = append(products, Data{Title: title, Body: body})
				//pageStatusMessage = fmt.Sprintf("Успех: %s", LimitString(title, 60))
				//statusMessageColor = ColorGreen
				parsedSuccessfully = true
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
		// Removed: fmt.Printf("\n%s[INFO] Парсинг статей RIA.ru завершен. Собрано %d статей.%s\n", ColorGreen, len(products), ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[RIA]%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 { // No products collected, but links were attempted
		fmt.Printf("\n%s[RIA]%s[ERROR] Парсинг статей RIA.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[RIA]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
