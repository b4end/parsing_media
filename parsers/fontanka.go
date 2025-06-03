package parsers

import (
	"fmt"
	. "parsing_media/utils"
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

	_ = getLinksFontanka()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[FONTANKA]%s[INFO] Парсер Fontanka.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksFontanka() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	doc, err := GetHTML(fontankaURLNews)
	if err != nil {
		fmt.Printf("%s[FONTANKA]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, fontankaURLNews, err, ColorReset)
		return getPageFontanka(foundLinks)
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
		return products
	}

	for _, pageURL := range links {
		var title, body string
		var tags []string
		var parsDate time.Time
		parsedSuccessfully := false

		doc, err := GetHTML(pageURL)
		if err != nil {
			errItems = append(errItems, fmt.Sprintf("(ошибка GET: %s)", err.Error()))
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

			dateStr, exists := doc.Find("time.item_psvU3").Attr("datetime")
			if exists {
				parsedTime, err := time.Parse(time.RFC3339, dateStr)
				if err != nil {
					fmt.Printf("%s[FONTANKA]%s[WARNING] Ошибка парсинга даты из атрибута 'datetime': '%s' на %s: %v%s\n", ColorBlue, ColorYellow, dateStr, pageURL, err, ColorReset)
				} else {
					parsDate = parsedTime
				}
			} else {
				fmt.Printf("%s[FONTANKA]%s[WARNING] Атрибут 'datetime' не найден у тега 'time.item_psvU3' на %s%s\n", ColorBlue, ColorYellow, pageURL, ColorReset)
			}

			doc.Find("div.scrollableBlock_oYLvg a.tag_S1lW8").Each(func(_ int, s *goquery.Selection) {
				tagText := strings.TrimSpace(s.Text())
				if tagText != "" {
					tags = append(tags, tagText)
				}
			})

			if title != "" && body != "" && !parsDate.IsZero() {
				products = append(products, Data{
					Href:  pageURL,
					Title: title,
					Body:  body,
					Date:  parsDate,
					Tags:  tags,
				})
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
			if len(tags) == 0 {
				reasons = append(reasons, "Tags:false")
			}
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", pageURL, strings.Join(reasons, ", ")))
		}
	}

	if len(products) > 0 {
		if len(errItems) > 0 {
			fmt.Printf("%s[FONTANKA]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[FONTANKA]%s[ERROR] Парсинг статей Fontanka.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[FONTANKA]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products
}
