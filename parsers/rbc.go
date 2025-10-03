package parsers

import (
	"encoding/json"
	"fmt"
	"net/http"
	. "parsing_media/utils"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	rbcURL         = "https://www.rbc.ru"
	rbcNewsPageURL = "https://www.rbc.ru/"
	numWorkersRbc  = 10
)

type RbcNextData struct {
	Props struct {
		PageProps struct {
			ArticleItem struct {
				Title             string `json:"title"`
				BodyMd            string `json:"bodyMd"`
				PublishDateT      int64  `json:"publishDateT"`
				ModifDateT        int64  `json:"modifDateT"`
				FirstPublishDateT int64  `json:"firstPublishDateT"`
				Tags              []struct {
					Title string `json:"title"`
				} `json:"tags"`
			} `json:"articleItem"`
		} `json:"pageProps"`
	} `json:"props"`
}

func RbcMain() {
	totalStartTime := time.Now()
	articles, links := getLinksRbc()
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[RBC]%s[INFO] Парсер RBC.ru заверщил работу собрав (%d/%d): (%s)%s\n", ColorBlue, ColorYellow, len(articles), len(links), FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksRbc() ([]Data, []string) {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	linkSelector := ".js-news-feed-list a.news-feed__item"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, rbcNewsPageURL)
	if err != nil {
		fmt.Printf("%s[RBC]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, rbcNewsPageURL, err, ColorReset)
		return getPageRbc(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if strings.HasPrefix(href, "https://www.rbc.ru/") {
				if idx := strings.Index(href, "?"); idx != -1 {
					href = href[:idx]
				}
				if !seenLinks[href] {
					if !strings.Contains(href, "editorial.rbc.ru") &&
						!strings.Contains(href, "rbc.group") &&
						!strings.Contains(href, "productstar.ru") &&
						!strings.Contains(href, "companies.rbc.ru") &&
						!strings.Contains(href, "ra.rbc.ru") {
						seenLinks[href] = true
						foundLinks = append(foundLinks, href)
					}
				}
			} else if (strings.HasPrefix(href, "https://www.rbc.ru/industries/") ||
				strings.HasPrefix(href, "https://www.rbc.ru/wine/") ||
				strings.HasPrefix(href, "https://trends.rbc.ru/trends/") ||
				strings.HasPrefix(href, "https://sportrbc.ru/") ||
				strings.HasPrefix(href, "https://pro.rbc.ru/") ||
				strings.HasPrefix(href, "https://realty.rbc.ru/")) && !strings.Contains(href, "?from=newsfeed") {
				if idx := strings.Index(href, "?"); idx != -1 {
					href = href[:idx]
				}
				if !seenLinks[href] {
					if !strings.Contains(href, "editorial.rbc.ru") &&
						!strings.Contains(href, "rbc.group") &&
						!strings.Contains(href, "productstar.ru") &&
						!strings.Contains(href, "companies.rbc.ru") &&
						!strings.Contains(href, "ra.rbc.ru") {
						seenLinks[href] = true
						foundLinks = append(foundLinks, href)
					}
				}
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[RBC]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, rbcNewsPageURL, ColorReset)
	}

	return getPageRbc(foundLinks)
}

type pageParseResultRbc struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func markdownToPlainText(md string) string {
	reLink := regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`)
	text := reLink.ReplaceAllString(md, "$1")

	reBold1 := regexp.MustCompile(`\*\*([^\*]+)\*\*`)
	text = reBold1.ReplaceAllString(text, "$1")
	reBold2 := regexp.MustCompile(`__([^_]+)__`)
	text = reBold2.ReplaceAllString(text, "$1")
	reItalic1 := regexp.MustCompile(`\*([^\*]+)\*`)
	text = reItalic1.ReplaceAllString(text, "$1")
	reItalic2 := regexp.MustCompile(`_([^_]+)_`)
	text = reItalic2.ReplaceAllString(text, "$1")

	text = strings.ReplaceAll(text, "«", "\"")
	text = strings.ReplaceAll(text, "»", "\"")
	text = strings.ReplaceAll(text, "—", "—")
	text = strings.ReplaceAll(text, " ", " ")

	reHTML := regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
	text = reHTML.ReplaceAllString(text, "")

	text = strings.ReplaceAll(text, "\n\n", "<TEMP_PARAGRAPH>")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "<TEMP_PARAGRAPH>", "\n\n")

	text = strings.TrimSpace(text)
	return text
}

func getPageRbc(links []string) ([]Data, []string) {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products, links
	}

	tagsAreMandatory := false

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersRbc + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersRbc,
		},
	}

	resultsChan := make(chan pageParseResultRbc, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersRbc
	if totalLinks < numWorkersRbc {
		actualNumWorkers = totalLinks
	}

	for i := 0; i < actualNumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pageURL := range linkChan {
				var title, body string
				var tags []string
				var parsDate time.Time
				var dateParseError error

				doc, err := GetHTMLForClient(httpClient, pageURL)
				if err != nil {
					resultsChan <- pageParseResultRbc{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				nextDataScript := doc.Find("script#__NEXT_DATA__").First()
				if nextDataScript.Length() > 0 {
					jsonData := nextDataScript.Text()
					var rbcData RbcNextData
					err = json.Unmarshal([]byte(jsonData), &rbcData)
					if err == nil && rbcData.Props.PageProps.ArticleItem.Title != "" {
						title = strings.TrimSpace(rbcData.Props.PageProps.ArticleItem.Title)
						body = markdownToPlainText(rbcData.Props.PageProps.ArticleItem.BodyMd)

						timestamp := rbcData.Props.PageProps.ArticleItem.PublishDateT
						if rbcData.Props.PageProps.ArticleItem.FirstPublishDateT != 0 {
							timestamp = rbcData.Props.PageProps.ArticleItem.FirstPublishDateT
						}
						if timestamp > 0 {
							parsDate = time.Unix(timestamp, 0)
						} else {
							dateParseError = fmt.Errorf("timestamp из __NEXT_DATA__ равен 0")
						}

						for _, tagItem := range rbcData.Props.PageProps.ArticleItem.Tags {
							if tagItem.Title != "" {
								tags = append(tags, strings.TrimSpace(tagItem.Title))
							}
						}
					} else if err != nil {
						fmt.Printf("%s[RBC]%s[DEBUG] Ошибка парсинга JSON из __NEXT_DATA__ для %s: %v%s\n", ColorBlue, ColorYellow, pageURL, err, ColorReset)
					}
				}

				if title == "" {
					title = strings.TrimSpace(doc.Find(".article__header__title-in").First().Text())
				}
				if title == "" {
					title = strings.TrimSpace(doc.Find("h1.article__header__title-in").First().Text())
				}
				if title == "" {
					title = strings.TrimSpace(doc.Find("h1.article__title").First().Text())
				}
				if title == "" {
					title = strings.TrimSpace(doc.Find("h1.article-title").First().Text())
				}
				if title == "" {
					title = strings.TrimSpace(doc.Find("h1.article-entry-title").First().Text())
				}

				if body == "" {
					var bodyBuilder strings.Builder
					doc.Find(".article__text p, .article__text_free p, .article-body__content p, .l-col-main .article__content p, .article-item-content p.paragraph, div[itemprop='articleBody'] p").Each(func(j int, s *goquery.Selection) {
						if s.Find("a[href*='t.me']").Length() > 0 && strings.Contains(s.Text(), "Читайте РБК в Telegram") {
							return
						}
						if s.Closest("figure").Length() > 0 || s.Closest(".article__inline-item").Length() > 0 || s.Closest(".article__inline-video").Length() > 0 || s.Closest(".styles_container__0VbDM").Length() > 0 {
							return
						}
						currentTextPart := strings.TrimSpace(s.Text())
						if currentTextPart != "" {
							if bodyBuilder.Len() > 0 {
								bodyBuilder.WriteString("\n\n")
							}
							bodyBuilder.WriteString(currentTextPart)
						}
					})
					body = bodyBuilder.String()
				}

				if parsDate.IsZero() {
					dateTextRaw := ""
					dateNode := doc.Find("time.article__header__date").First()
					if dateNode.Length() > 0 {
						dateTextRaw, _ = dateNode.Attr("datetime")
					}
					if dateTextRaw == "" {
						dateNode = doc.Find(".article__header__date .article__header__date-text").First()
						if dateNode.Length() > 0 {
							dateTextRaw, _ = dateNode.Attr("content")
						}
					}
					if dateTextRaw == "" {
						dateNode = doc.Find("meta[itemprop='datePublished']").First()
						if dateNode.Length() > 0 {
							dateTextRaw, _ = dateNode.Attr("content")
						}
					}
					if dateTextRaw == "" {
						dateNode = doc.Find(".article-entry-meta .meta-info-row-date").First()
						dateTextRaw = dateNode.Text()
						if dateTextRaw != "" {
							currentYear := time.Now().Year()
							fullDateStr := fmt.Sprintf("%s %d", dateTextRaw, currentYear)

							for rus, eng := range RussianMonths {
								fullDateStr = strings.Replace(fullDateStr, rus, eng, 1)
							}
							fullDateStr = strings.Replace(fullDateStr, ",", "", 1)

							layout := "02 01 15:04 2006"
							locationMSK := time.FixedZone("MSK", 3*60*60)
							parsedTime, parseErr := time.ParseInLocation(layout, fullDateStr, locationMSK)
							if parseErr == nil {
								parsDate = parsedTime
							} else {
								dateParseError = fmt.Errorf("ошибка парсинга даты нового формата '%s': %v", dateTextRaw, parseErr)
							}
						}
					}

					dateToParse := strings.TrimSpace(dateTextRaw)
					if dateToParse != "" && parsDate.IsZero() {
						parsedTime, parseErr := time.Parse(time.RFC3339, dateToParse)
						if parseErr != nil {
							dateParseError = fmt.Errorf("не удалось спарсить RFC3339 '%s': %v", dateToParse, parseErr)
						} else {
							parsDate = parsedTime
						}
					} else if dateToParse == "" && parsDate.IsZero() {
						dateParseError = fmt.Errorf("строка даты пуста (старый формат)")
					}
				}

				if len(tags) == 0 {
					doc.Find(".article__tags__container a.article__tags__item, .article__tags a.article__tag, .tags__list a.tags__link, .tabs-content a.tag").Each(func(_ int, s *goquery.Selection) {
						tagText := strings.TrimSpace(s.Text())
						if tagText != "" {
							tags = append(tags, tagText)
						}
					})
				}

				if dateParseError != nil && !parsDate.IsZero() {
					dateParseError = nil
				}

				if dateParseError != nil && parsDate.IsZero() {
					fmt.Printf("%s[RBC]%s[WARNING] Ошибка парсинга даты на %s: %v%s\n", ColorBlue, ColorYellow, pageURL, dateParseError, ColorReset)
				}

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					dataItem := Data{
						Site:  rbcURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultRbc{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultRbc{Data: dataItem}
				} else {
					var reasons []string
					if title == "" {
						reasons = append(reasons, "T:false")
					}
					if body == "" {
						reasons = append(reasons, "B:false")
					}
					if parsDate.IsZero() {
						reasonDate := "D:false"
						if dateParseError != nil {
							reasonDate = fmt.Sprintf("D:false (err: %v)", dateParseError)
						} else {
							reasonDate = "D:false (empty_str_or_parsing_failed_silently)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tags) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultRbc{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for result := range resultsChan {
		if result.Error != nil {
			errItems = append(errItems, fmt.Sprintf("%s (%s)", result.PageURL, result.Error.Error()))
		} else if result.IsEmpty {
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", result.PageURL, strings.Join(result.Reasons, ", ")))
		} else {
			products = append(products, result.Data)
		}
	}

	if len(products) > 0 {
		if len(errItems) > 0 {
			fmt.Printf("%s[RBC]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[RBC]%s[ERROR] Парсинг статей RBC.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[RBC]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	return products, links
}
