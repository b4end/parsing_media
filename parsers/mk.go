package parsers

import (
	"fmt"
	. "parsing_media/utils" // Assuming Data, ColorBlue, ColorYellow, ColorRed, ColorReset, GetHTML, FormatDuration, LimitString, GenerateURLForPastDate are defined here
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	mkURL         = "https://www.mk.ru"
	mkNewsPageURL = "https://www.mk.ru/news/" // URL for the main news listing page
)

func MKMain() {
	totalStartTime := time.Now()

	// Removed: fmt.Printf("%s[INFO] Запуск парсера MK.ru...%s\n", ColorYellow, ColorReset)
	_ = getLinksMK() // Assuming we don't need the result for now

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[MK]%s[INFO] Общее время выполнения парсера MK.ru: %s%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksMK() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	targetURL := mkNewsPageURL // Use the main news listing page URL

	// Removed: fmt.Printf("%s[INFO] Начало парсинга ссылок с %s (новости с главной новостной страницы)...%s\n", ColorYellow, targetURL, ColorReset)
	// Minimal logging is preferred as per the original template.

	doc, err := GetHTML(targetURL)
	if err != nil {
		fmt.Printf("%s[MK]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, targetURL, err, ColorReset)
		return getPageMK(foundLinks) // Proceed with empty links
	}

	doc.Find("a.news-listing__item-link").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			// Check if the link is an advertisement
			// Ads often have an h3 child with class 'news-listing__item_ad'
			isAd := s.Find("h3.news-listing__item_ad").Length() > 0

			if !isAd && strings.HasPrefix(href, mkURL) { // Ensure it's an absolute mkURL and not an ad
				if !seenLinks[href] {
					seenLinks[href] = true
					foundLinks = append(foundLinks, href)
				}
			}
		}
	})

	if len(foundLinks) > 0 {
		// Removed: fmt.Printf("%s[INFO] Найдено %d уникальных ссылок на статьи (с новостной страницы).%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("%s[MK]%s[WARNING] Не найдено ссылок с селектором 'a.news-listing__item-link' на странице %s (или все найденные ссылки являются рекламными).%s\n", ColorBlue, ColorYellow, targetURL, ColorReset)
	}

	return getPageMK(foundLinks[:50])
}

func getPageMK(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		// Removed: fmt.Printf("%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorYellow, ColorReset)
		return products
	}
	// Removed: fmt.Printf("\n%s[INFO] Начало парсинга %d статей с MK.ru...%s\n", ColorYellow, totalLinks, ColorReset)

	for _, pageURL := range links {
		var title, body string
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("%s (ошибка GET: %s)", LimitString(pageURL, 60), LimitString(err.Error(), 50)))
		} else {
			title = strings.TrimSpace(doc.Find("h1.article__title").First().Text())
			var bodyBuilder strings.Builder
			doc.Find("div.article__body p").Each(func(_ int, s *goquery.Selection) {
				partText := strings.TrimSpace(s.Text())
				if partText != "" {
					if strings.Contains(partText, "Самые яркие фото и видео дня") && strings.Contains(partText, "Telegram-канале") {
						return
					}

					if bodyBuilder.Len() > 0 {
						bodyBuilder.WriteString("\n\n")
					}
					bodyBuilder.WriteString(partText)
				}
			})
			body = bodyBuilder.String()

			if title != "" && body != "" {
				products = append(products, Data{Title: title, Body: body})
				parsedSuccessfully = true
			}
		}

		if !parsedSuccessfully && err == nil { // Only add to errItems if GetHTML was successful but data extraction failed
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: T:%t, B:%t)", LimitString(pageURL, 60), title != "", body != ""))
		}
	}

	if len(products) > 0 {
		// Removed: fmt.Printf("\n%s[INFO] Парсинг статей MK.ru завершен. Собрано %d статей.%s\n", ColorGreen, len(products), ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[MK]%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 { // No products collected, but links were attempted
		fmt.Printf("\n%s[MK]%s[ERROR] Парсинг статей MK.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[MK]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
