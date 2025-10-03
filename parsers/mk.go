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
	mkURL         = "https://www.mk.ru"
	mkNewsPageURL = "https://www.mk.ru/news/"
	mkAutoURL     = "https://www.mk.ru/auto/"
	numWorkersMK  = 10
	mkDateLayout  = "2006-01-02T15:04:05-0700"
)

func MKMain() {
	totalStartTime := time.Now()
	articles, links := getLinksMK()
	SaveData(articles)
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[MK]%s[INFO] Парсер MK.ru заверщил работу собрав (%d/%d): (%s)%s\n", ColorBlue, ColorYellow, len(articles), len(links), FormatDuration(totalElapsedTime), ColorReset)
}

func getLinksMK() ([]Data, []string) {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	targetURL := mkNewsPageURL
	linkSelector := "a.news-listing__item-link"

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	doc, err := GetHTMLForClient(client, targetURL)
	if err != nil {
		fmt.Printf("%s[MK]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, targetURL, err, ColorReset)
		return getPageMK(foundLinks)
	}

	doc.Find(linkSelector).Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			isAd := s.Find("h3.news-listing__item_ad").Length() > 0
			if !isAd && strings.HasPrefix(href, mkURL) && !strings.HasPrefix(href, mkAutoURL) {
				if !seenLinks[href] {
					seenLinks[href] = true
					foundLinks = append(foundLinks, href)
				}
			}
		}
	})

	if len(foundLinks) == 0 {
		fmt.Printf("%s[MK]%s[WARNING] Не найдено ссылок с селектором '%s' на странице %s (или все найденные ссылки являются рекламными/автомобильными).%s\n", ColorBlue, ColorYellow, linkSelector, targetURL, ColorReset)
	}

	limit := 50
	if len(foundLinks) < limit {
		limit = len(foundLinks)
	}
	return getPageMK(foundLinks[:limit])
}

type pageParseResultMK struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

var (
	russianMonthsGenitive = map[string]time.Month{
		"января":   time.January,
		"февраля":  time.February,
		"марта":    time.March,
		"апреля":   time.April,
		"мая":      time.May,
		"июня":     time.June,
		"июля":     time.July,
		"августа":  time.August,
		"сентября": time.September,
		"октября":  time.October,
		"ноября":   time.November,
		"декабря":  time.December,
	}
	altDateRegex = regexp.MustCompile(`^(\d{1,2})\s+([а-яА-Я]+),?\s+(\d{2}:\d{2})$`)
)

func parseHHMM(hhmmStr string) (hour, minute int, err error) {
	parts := strings.Split(hhmmStr, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid HH:MM format: '%s', expected 2 parts, got %d", hhmmStr, len(parts))
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid hour in HH:MM string '%s': %w", hhmmStr, err)
	}
	minute, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minute in HH:MM string '%s': %w", hhmmStr, err)
	}

	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("time out of range in HH:MM string '%s': H=%d, M=%d", hhmmStr, hour, minute)
	}
	return hour, minute, nil
}

func parseRelativeTime(timeStr string, referenceTime time.Time, dayOffset int, loc *time.Location) (time.Time, error) {
	hour, minute, err := parseHHMM(timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("relative time parse: %w", err)
	}

	targetDay := referenceTime.AddDate(0, 0, dayOffset)
	return time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day(), hour, minute, 0, 0, loc), nil
}

func parseMkDate(dateString string) (time.Time, error) {
	t, err := time.Parse(mkDateLayout, dateString)
	if err == nil {
		return t, nil
	}
	originalErr := err

	loc, locErr := time.LoadLocation("Europe/Moscow")
	if locErr != nil {
		loc = time.FixedZone("MSK", 3*60*60)
	}
	now := time.Now().In(loc)

	lowerDateString := strings.ToLower(dateString)
	if strings.HasPrefix(lowerDateString, "сегодня,") {
		timeStr := strings.TrimSpace(dateString[len("сегодня,"):])
		parsedTime, err := parseRelativeTime(timeStr, now, 0, loc)
		if err == nil {
			return parsedTime, nil
		}
	} else if strings.HasPrefix(lowerDateString, "сегодня ") {
		timeStr := strings.TrimSpace(dateString[len("сегодня "):])
		parsedTime, err := parseRelativeTime(timeStr, now, 0, loc)
		if err == nil {
			return parsedTime, nil
		}
	}

	if strings.HasPrefix(lowerDateString, "вчера,") {
		timeStr := strings.TrimSpace(dateString[len("вчера,"):])
		parsedTime, err := parseRelativeTime(timeStr, now, -1, loc)
		if err == nil {
			return parsedTime, nil
		}
	} else if strings.HasPrefix(lowerDateString, "вчера ") {
		timeStr := strings.TrimSpace(dateString[len("вчера "):])
		parsedTime, err := parseRelativeTime(timeStr, now, -1, loc)
		if err == nil {
			return parsedTime, nil
		}
	}

	matches := altDateRegex.FindStringSubmatch(dateString)
	if len(matches) == 4 {
		dayStr := matches[1]
		monthNameStr := strings.ToLower(matches[2])
		timeStr := matches[3]

		day, errDay := strconv.Atoi(dayStr)
		if errDay != nil {
			return time.Time{}, fmt.Errorf("alt date parse: invalid day '%s' from '%s'", dayStr, dateString)
		}

		month, monthFound := russianMonthsGenitive[monthNameStr]
		if !monthFound {
			return time.Time{}, fmt.Errorf("alt date parse: unknown month '%s' from '%s'", monthNameStr, dateString)
		}

		hour, minute, errTime := parseHHMM(timeStr)
		if errTime != nil {
			return time.Time{}, fmt.Errorf("alt date parse: invalid time '%s' from '%s': %w", timeStr, dateString, errTime)
		}

		currentYear := now.Year()
		parsedTime := time.Date(currentYear, month, day, hour, minute, 0, 0, loc)

		if parsedTime.After(now.Add(24 * time.Hour)) {
			parsedTime = time.Date(currentYear-1, month, day, hour, minute, 0, 0, loc)
		}
		return parsedTime, nil
	}

	return time.Time{}, fmt.Errorf("failed to parse date '%s' with standard layout (err: %v) or any alternative format", dateString, originalErr)
}

func getPageMK(links []string) ([]Data, []string) {
	var products []Data
	var errItems []string
	totalLinks := len(links)

	if totalLinks == 0 {
		return products, links
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: numWorkersMK + 5,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     numWorkersMK,
		},
	}

	resultsChan := make(chan pageParseResultMK, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersMK
	if totalLinks < numWorkersMK {
		actualNumWorkers = totalLinks
	}

	for i := 0; i < actualNumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pageURL := range linkChan {
				var title, body string
				var parsDate time.Time
				var tags []string

				doc, err := GetHTMLForClient(httpClient, pageURL)
				if err != nil {
					resultsChan <- pageParseResultMK{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				title = strings.TrimSpace(doc.Find("h1.article__title").First().Text())

				var bodyBuilder strings.Builder
				doc.Find("div.article__body p").Each(func(_ int, s *goquery.Selection) {
					partText := strings.TrimSpace(s.Text())
					if partText != "" {
						if strings.Contains(partText, "Самые яркие фото и видео дня") && strings.Contains(partText, "Telegram-канале") {
							return
						}
						if (strings.HasPrefix(partText, "Читайте также:") || strings.HasPrefix(partText, "Смотрите видео по теме:")) && s.Find("a").Length() > 0 {
							return
						}
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(partText)
					}
				})
				body = bodyBuilder.String()

				var dateParseErrorMessage string
				dateString, exists := doc.Find("time.meta__text[datetime]").Attr("datetime")
				if exists && dateString != "" {
					var parseErr error
					parsDate, parseErr = parseMkDate(dateString)
					if parseErr != nil {
						dateParseErrorMessage = fmt.Sprintf("исходная строка: '%s', ошибка: %v", dateString, parseErr)
					}
				} else {
					dateParseErrorMessage = "атрибут datetime отсутствует или пуст"
				}

				if title != "" && body != "" && !parsDate.IsZero() {
					dataItem := Data{
						Site:  mkURL,
						Href:  pageURL,
						Title: title,
						Body:  body,
						Date:  parsDate,
						Tags:  tags,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultMK{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultMK{Data: dataItem}
				} else {
					var reasons []string
					if title == "" {
						reasons = append(reasons, "T:false (пустой заголовок)")
					}
					if body == "" {
						reasons = append(reasons, "B:false (пустое тело статьи)")
					}
					if parsDate.IsZero() {
						reasonMsg := "D:false"
						if dateParseErrorMessage != "" {
							reasonMsg += " (" + dateParseErrorMessage + ")"
						} else if dateString != "" {
							reasonMsg += " (исходная строка: '" + dateString + "')"
						} else {
							reasonMsg += " (атрибут datetime не найден или пуст)"
						}
						reasons = append(reasons, reasonMsg)
					}
					resultsChan <- pageParseResultMK{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
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
			fmt.Printf("%s[MK]%s[WARNING] Не удалось полностью обработать %d из %d страниц (или отсутствовали некоторые данные):%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("%s[MK]%s[ERROR] Парсинг статей MK.ru завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		if len(errItems) > 0 {
			fmt.Printf("%s[MK]%s[INFO] Список страниц с ошибками или без данных:%s\n", ColorBlue, ColorYellow, ColorReset)
			for idx, itemMessage := range errItems {
				fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
			}
		}
	}

	return products, links
}
