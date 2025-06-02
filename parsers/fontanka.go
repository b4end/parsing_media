package parsers

import (
	"fmt"
	. "parsing_media/utils" // Assuming Data, ColorBlue, ColorYellow, ColorRed, ColorReset, GetHTML, FormatDuration, LimitString are defined here
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	fontankaURL     = "https://www.fontanka.ru"
	fontankaURLNews = "https://www.fontanka.ru/politic/"
)

func FontankaMain() {
	totalStartTime := time.Now()

	_ = getLinksFontanka() // Assuming we don't need the result for now, like in BankiMain

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[FONTANKA]%s[INFO] Общее время выполнения парсера Fontanka.ru: %s%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksFontanka() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	doc, err := GetHTML(fontankaURLNews)
	if err != nil {
		fmt.Printf("%s[FONTANKA]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, fontankaURLNews, err, ColorReset)
		return getPageFontanka(foundLinks) // Proceed with empty links
	}

	doc.Find("a.header_RL97A").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullURL := ""
			if strings.HasPrefix(href, fontankaURL) {
				fullURL = href
			} else if strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//") {
				fullURL = fontankaURL + href
			}

			if fullURL != "" && !seenLinks[fullURL] {
				seenLinks[fullURL] = true
				foundLinks = append(foundLinks, fullURL)
			}
		}
	})

	if len(foundLinks) <= 0 {
		fmt.Printf("%s[FONTANKA]%s[WARNING] Не найдено ссылок с селектором 'a.header_RL97A' на странице %s.%s\n", ColorBlue, ColorYellow, fontankaURLNews, ColorReset)
	}

	return getPageFontanka(foundLinks)
}

func getPageFontanka(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		// Following Banki.go template, no message here.
		// Warning/error about no links should come from getLinksFontanka.
		return products
	}

	for _, pageURL := range links {
		var title, body string
		////var pageStatusMessage string
		////var statusMessageColor = ColorReset
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			//pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", LimitString(err.Error(), 50))
			//statusMessageColor = ColorRed
			errItems = append(errItems, fmt.Sprintf("%s (ошибка GET: %s)", LimitString(pageURL, 60), LimitString(err.Error(), 50)))
		} else {
			title = strings.TrimSpace(doc.Find("h1[class*='title_BgFsr']").First().Text())

			var bodyBuilder strings.Builder
			doc.Find("div.uiArticleBlockText_5xJo1.text-style-body-1.c-text.block_0DdLJ").Find("p, a, li, blockquote").Each(func(_ int, s *goquery.Selection) {

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
		if len(errItems) > 0 {
			fmt.Printf("%s[FONTANKA]%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset) // This formatting for individual items is consistent with Banki
			}
		}
	} else if totalLinks > 0 { // No products collected, but links were attempted
		fmt.Printf("\n%s[FONTANKA]%s[ERROR] Парсинг статей Fontanka.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[FONTANKA]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset) // This formatting for individual items is consistent with Banki
			}
		}
	}
	// If totalLinks == 0 initially, no summary message about processing is needed here.

	return products
}
