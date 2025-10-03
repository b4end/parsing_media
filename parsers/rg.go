package parsers

import (
	"fmt"
	"net/http"
	. "parsing_media/utils"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	rgURL         = "https://rg.ru"
	rgNewsPageURL = "https://rg.ru/news.html"
	numWorkersRG  = 10
)

func RGMain() {
	totalStartTime := time.Now()
	articles, links := getLinksRG()
	SaveData(articles)
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[RG]%s[INFO] Парсер RG.ru заверщил работу собрав (%d/%d): (%s)%s\n", ColorBlue, ColorYellow, len(articles), len(links), FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksRG() ([]Data, []string) {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	linkSelector := "ul.PageNewsContent_list__P3OgM li.PageNewsContent_item__NmJXl a.PageNewsContentItem_root__oascP"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, rgNewsPageURL)
	if err != nil {
		fmt.Printf("%s[RG]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, rgNewsPageURL, err, ColorReset)
		return getPageRG(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			fullURL := ""
			if strings.HasPrefix(href, "/") {
				if !strings.HasPrefix(href, "//rodina-history.rg.ru") && !strings.Contains(href, "rodina-history.ru") {
					fullURL = rgURL + href
				}
			} else if strings.HasPrefix(href, rgURL) {
				fullURL = href
			} else if strings.HasPrefix(href, "https://rodina-history.ru") {
				return
			}

			if fullURL != "" {
				if idx := strings.Index(fullURL, "?"); idx != -1 {
					fullURL = fullURL[:idx]
				}
				if !seenLinks[fullURL] {
					seenLinks[fullURL] = true
					foundLinks = append(foundLinks, fullURL)
				}
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[RG]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, rgNewsPageURL, ColorReset)
	}

	return getPageRG(foundLinks)
}

type pageParseResultRG struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func getPageRG(links []string) ([]Data, []string) {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products, links
	}

	dateLayout := "02.01.2006 15:04"
	locationMSK := time.FixedZone("MSK", 3*60*60)
	tagsAreMandatory := false

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersRG + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersRG,
		},
	}

	resultsChan := make(chan pageParseResultRG, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersRG
	if totalLinks < numWorkersRG {
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
					resultsChan <- pageParseResultRG{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1.PageArticleCommonTitle_title__fUDQW").First().Text())

				var bodyBuilder strings.Builder
				doc.Find("div.PageContentCommonStyling_text__CKOzO p").Each(func(j int, s *goquery.Selection) {
					if s.Closest("rg-incut").Length() > 0 || s.Closest("figure").Length() > 0 || s.Closest(".Likes_wrapper__paVes").Length() > 0 {
						return
					}
					if strings.TrimSpace(s.Text()) == "" && s.Children().Length() == 0 {
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

				dateTextRaw := strings.TrimSpace(doc.Find("div.ContentMetaDefault_date__wS0te").First().Text())
				dateToParse := dateTextRaw

				if dateToParse != "" {
					parsedTime, parseErr := time.ParseInLocation(dateLayout, dateToParse, locationMSK)
					if parseErr != nil {
						dateParseError = fmt.Errorf("ошибка парсинга даты '%s' (формат '%s'): %v", dateToParse, dateLayout, parseErr)
					} else {
						parsDate = parsedTime
					}
				} else {
					metaDate, metaDateExists := doc.Find("meta[property='article:published_time']").Attr("content")
					if metaDateExists {
						parsedTime, parseErr := time.Parse(time.RFC3339, metaDate)
						if parseErr == nil {
							parsDate = parsedTime.In(locationMSK)
						} else {
							dateParseError = fmt.Errorf("ошибка парсинга мета-даты '%s': %v", metaDate, parseErr)
						}
					} else {
						dateParseError = fmt.Errorf("строка даты и мета-дата пусты")
					}
				}

				if !parsDate.IsZero() {
					parsDate = parsDate.In(locationMSK)
					dateParseError = nil
				}

				doc.Find(".EditorialTags_tags__7zYTH a .EditorialTags_tag__BMT4K").Each(func(_ int, s *goquery.Selection) {
					tagText := strings.TrimSpace(s.Text())
					if strings.HasPrefix(tagText, "#") {
						tagText = strings.TrimPrefix(tagText, "#")
					}
					if tagText != "" {
						tags = append(tags, tagText)
					}
				})
				if len(tags) == 0 {
					doc.Find(".PageArticleContent_relationBottom__jIiqg a[class*='LinksOfRubric_item'], .PageArticleContent_relationBottom__jIiqg a[class*='LinksOfSujet_item']").Each(func(_ int, s *goquery.Selection) {
						tagText := strings.TrimSpace(s.Text())
						if tagText != "" {
							tags = append(tags, tagText)
						}
					})
				}

				if len(tags) > 0 {
					seenTags := make(map[string]bool)
					uniqueTags := []string{}
					for _, tag := range tags {
						if !seenTags[tag] {
							seenTags[tag] = true
							uniqueTags = append(uniqueTags, tag)
						}
					}
					tags = uniqueTags
				}

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					dataItem := Data{
						Site:  rgURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultRG{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultRG{Data: dataItem}
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
							reasonDate = fmt.Sprintf("D:false (err: %v, str: '%s')", dateParseError, dateToParse)
						} else if dateToParse == "" {
							reasonDate = "D:false (empty_str)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tags) == 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultRG{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
			fmt.Printf("%s[RG]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[RG]%s[ERROR] Парсинг статей RG.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[RG]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}
	return products, links
}
