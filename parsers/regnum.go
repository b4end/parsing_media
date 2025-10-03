package parsers

import (
	"fmt"
	"net/http"
	. "parsing_media/utils"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	regnumURL         = "https://regnum.ru"
	regnumNewsPageURL = "https://regnum.ru/news"
	numWorkersRegnum  = 10
)

var russianMonthsRegnum = map[string]string{
	"января":   "January",
	"февраля":  "February",
	"марта":    "March",
	"апреля":   "April",
	"мая":      "May",
	"июня":     "June",
	"июля":     "July",
	"августа":  "August",
	"сентября": "September",
	"октября":  "October",
	"ноября":   "November",
	"декабря":  "December",
}

func RegnumMain() {
	totalStartTime := time.Now()
	articles, links := getLinksRegnum()
	SaveData(articles)
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[REGNUM]%s[INFO] Парсер Regnum.ru заверщил работу собрав (%d/%d): (%s)%s\n", ColorBlue, ColorYellow, len(articles), len(links), FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksRegnum() ([]Data, []string) {
	var foundLinks []string
	seenLinks := make(map[string]bool)
	linkSelector := "div.news-item div.news-header a.title"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, regnumNewsPageURL)
	if err != nil {
		fmt.Printf("%s[REGNUM]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, regnumNewsPageURL, err, ColorReset)
		return getPageRegnum(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if strings.HasPrefix(href, "/news/") {
				fullLink := regnumURL + href
				if !seenLinks[fullLink] {
					seenLinks[fullLink] = true
					foundLinks = append(foundLinks, fullLink)
				}
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[REGNUM]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s.%s\n", ColorBlue, ColorYellow, linkSelector, regnumNewsPageURL, ColorReset)
	}

	return getPageRegnum(foundLinks)
}

type pageParseResultRegnum struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

func parseRegnumDate(dateStr string, loc *time.Location) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)
	parts := strings.Split(dateStr, ",")
	if len(parts) < 3 {
		return time.Time{}, fmt.Errorf("неверный формат строки даты, ожидалось как минимум 3 части через запятую: '%s'", dateStr)
	}

	dayMonthPart := strings.TrimSpace(parts[0])
	yearPart := strings.TrimSpace(parts[1])
	timePart := strings.TrimSpace(parts[2])

	dayMonthFields := strings.Fields(dayMonthPart)
	if len(dayMonthFields) != 2 {
		return time.Time{}, fmt.Errorf("не удалось разделить день и месяц из '%s'", dayMonthPart)
	}
	dayStr := dayMonthFields[0]
	monthRu := dayMonthFields[1]

	monthEn, ok := russianMonthsRegnum[strings.ToLower(monthRu)]
	if !ok {
		return time.Time{}, fmt.Errorf("неизвестный русский месяц: '%s'", monthRu)
	}

	year, err := strconv.Atoi(yearPart)
	if err != nil {
		return time.Time{}, fmt.Errorf("не удалось преобразовать год '%s' в число: %w", yearPart, err)
	}

	fullDateToParse := fmt.Sprintf("%s %s %d %s", dayStr, monthEn, year, timePart)
	layout := "2 January 2006 15:04"

	parsedTime, err := time.ParseInLocation(layout, fullDateToParse, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("ошибка парсинга даты '%s' форматом '%s': %w", fullDateToParse, layout, err)
	}
	return parsedTime, nil
}

func getPageRegnum(links []string) ([]Data, []string) {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products, links
	}

	tagsAreMandatory := false
	locationMSK := time.FixedZone("MSK", 3*60*60)

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersRegnum + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersRegnum,
		},
	}

	resultsChan := make(chan pageParseResultRegnum, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersRegnum
	if totalLinks < numWorkersRegnum {
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
				var dateStringToParse string

				doc, err := GetHTMLForClient(httpClient, pageURL)
				if err != nil {
					resultsChan <- pageParseResultRegnum{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1.article-header").First().Text())

				articleTextNode := doc.Find("div.article-text").First()

				dateInfoLine := articleTextNode.Find("p span.article-info-line").First().Text()
				if dateInfoLine != "" {
					re := regexp.MustCompile(`,\s*(\d+\s+[а-яА-Я]+,\s*\d{4},\s*\d{2}:\d{2})`)
					matches := re.FindStringSubmatch(dateInfoLine)
					if len(matches) > 1 {
						dateStringToParse = strings.TrimSpace(matches[1])
						parsedTime, parseErr := parseRegnumDate(dateStringToParse, locationMSK)
						if parseErr != nil {
							dateParseError = parseErr
						} else {
							parsDate = parsedTime
						}
					} else {
						dateParseError = fmt.Errorf("не удалось извлечь дату из строки: '%s'", dateInfoLine)
					}
				} else {
					dateParseError = fmt.Errorf("не найден span.article-info-line с датой")
				}

				var bodyBuilder strings.Builder
				articleTextNode.Find("p").Each(func(idx int, pSelection *goquery.Selection) {
					if pSelection.Find("span.article-info-line").Length() > 0 {
						return
					}
					if pSelection.Find("div.picture-wrapper, div.adv-container-wrapper").Length() > 0 {
						return
					}

					currentTextPart := strings.TrimSpace(pSelection.Text())
					if currentTextPart != "" {
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(currentTextPart)
					}
				})
				body = bodyBuilder.String()

				if title != "" && body != "" && !parsDate.IsZero() && (!tagsAreMandatory || len(tags) > 0) {
					dataItem := Data{
						Site:  regnumURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultRegnum{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultRegnum{Data: dataItem}
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
							reasonDate = fmt.Sprintf("D:false (err: %v, str: '%s')", dateParseError, dateStringToParse)
						} else if dateStringToParse == "" {
							reasonDate = "D:false (empty_str_or_not_found)"
						}
						reasons = append(reasons, reasonDate)
					}
					if tagsAreMandatory && len(tags) > 0 {
						reasons = append(reasons, "Tags:false")
					}
					resultsChan <- pageParseResultRegnum{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	processedCount := 0
	for result := range resultsChan {
		processedCount++
		if result.Error != nil {
			errItems = append(errItems, fmt.Sprintf("%s (%s)", result.PageURL, result.Error.Error()))
		} else if result.IsEmpty {
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", result.PageURL, strings.Join(result.Reasons, ", ")))
		} else {
			products = append(products, result.Data)
		}
	}

	if len(errItems) > 0 {
		fmt.Printf("%s[REGNUM]%s[WARNING] Не удалось обработать %d из %d страниц (или отсутствовали данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
		maxErrorsToShow := 20
		for idx, itemMessage := range errItems {
			if idx < maxErrorsToShow {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			} else if idx == maxErrorsToShow {
				fmt.Printf("%s  ... и еще %d ошибок/предупреждений ...%s\n", ColorYellow, len(errItems)-maxErrorsToShow, ColorReset)
				break
			}
		}
	}

	if len(products) == 0 && totalLinks > 0 {
		fmt.Printf("%s[REGNUM]%s[ERROR] Парсинг статей Regnum.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
	}
	return products, links
}
