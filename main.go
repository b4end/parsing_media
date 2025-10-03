package main

import (
	"bufio"
	"fmt"
	"os"
	"parsing_media/parsers"
	. "parsing_media/utils"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ParserInfo struct {
	Name string
	Func func()
}

var ParserDefinitions = []ParserInfo{
	{Name: "RIA", Func: parsers.RiaMain},
	{Name: "Gazeta", Func: parsers.GazetaMain},
	{Name: "Lenta", Func: parsers.LentaMain},
	{Name: "Vesti", Func: parsers.VestiMain},
	{Name: "Kommersant", Func: parsers.KommersMain},
	{Name: "MK", Func: parsers.MKMain},
	{Name: "Fontanka", Func: parsers.FontankaMain},
	{Name: "Smotrim", Func: parsers.SmotrimMain},
	{Name: "Banki", Func: parsers.BankiMain},
	{Name: "DumaTV", Func: parsers.DumaTVMain},
	{Name: "RBC", Func: parsers.RbcMain},
	{Name: "Izvestiya", Func: parsers.IzMain},
	{Name: "Interfax", Func: parsers.InterfaxMain},
	{Name: "RG", Func: parsers.RGMain},
	{Name: "KP", Func: parsers.KPMain},
	{Name: "Ura", Func: parsers.UraMain},
	{Name: "Life", Func: parsers.LifeMain},
	{Name: "Regnum", Func: parsers.RegnumMain},
	{Name: "Tass", Func: parsers.TassMain},
	{Name: "Vedomosti", Func: parsers.VedomostiMain},
	{Name: "AIF", Func: parsers.AifMain},
}

func displayMenu() {
	fmt.Printf("\n%s--- МЕНЮ ЗАПУСКА ПАРСЕРОВ ---%s\n", ColorYellow, ColorReset)
	fmt.Printf("!  - Выход\n")
	fmt.Printf("0  - Запустить все циклично\n")
	fmt.Println(strings.Repeat("-", 50))

	const columns = 3
	for i, p := range ParserDefinitions {
		fmt.Printf("%2d - %-15s", i+1, p.Name)
		if (i+1)%columns == 0 || i == len(ParserDefinitions)-1 {
			fmt.Println()
		}
	}
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Ваш выбор: ")
}

func main() {

	// Инициализация соединения с БД
	fmt.Printf("%s[INFO] Инициализация соединения с базой данных...%s\n", ColorBlue, ColorReset)
	if err := InitDB(); err != nil {
		fmt.Printf("%s[FATAL] Ошибка подключения к БД: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	fmt.Printf("%s[DB] Соединение с БД установлено. Готовность к работе.%s\n", ColorBlue, ColorReset)

	reader := bufio.NewReader(os.Stdin)

	for {
		displayMenu()

		input, _ := reader.ReadString('\n')
		num := strings.TrimSpace(input)

		switch num {
		case "!":
			fmt.Printf("%s[INFO] Завершение работы.%s\n", ColorBlue, ColorReset)
			return
		case "0":
			runAllParsersInLoop(ParserDefinitions, reader)
		default:
			choice, err := strconv.Atoi(num)
			if err != nil || choice < 1 || choice > len(ParserDefinitions) {
				fmt.Printf("\n%s[ОШИБКА] Неверный ввод. Пожалуйста, выберите номер из списка.%s\n", ColorRed, ColorReset)
				time.Sleep(2 * time.Second)
				continue
			}

			selectedParser := ParserDefinitions[choice-1]
			fmt.Printf("\n%s[INFO] Запуск парсера: %s%s\n", ColorBlue, selectedParser.Name, ColorReset)
			selectedParser.Func()
			fmt.Printf("\n%s[INFO] Парсер %s завершил работу.%s\n", ColorBlue, selectedParser.Name, ColorReset)
		}
	}
}

func runAllParsersInLoop(parsers []ParserInfo, reader *bufio.Reader) {
	interruptChan := make(chan struct{})
	var interruptOnce sync.Once

	go func() {
		_, _ = reader.ReadString('\n')
		interruptOnce.Do(func() {
			close(interruptChan)
		})
	}()

	fmt.Printf("\n%s[INFO] Запуск всех парсеров в цикле. Нажмите Enter для остановки после текущей итерации.%s\n", ColorBlue, ColorReset)

	keepRunning := true
	for keepRunning {
		const totalWaitDuration = 3 * time.Minute
		// 1. Засекаем время окончания цикла СРАЗУ
		deadline := time.Now().Add(totalWaitDuration)

		fmt.Printf("\n%s[INFO] Запускаем новую итерацию... Следующий запуск в %s%s\n", ColorBlue, deadline.Format("15:04:05"), ColorReset)

		// 2. Запускаем все парсеры в фоне и получаем канал, который закроется по их завершению
		parsersDoneChan := make(chan struct{})
		go func() {
			var wg sync.WaitGroup
			for _, p := range parsers {
				wg.Add(1)
				go func(parserFunc func()) {
					defer wg.Done()
					parserFunc()
				}(p.Func)
			}
			wg.Wait()
			close(parsersDoneChan) // Сигналим о завершении
		}()

		// 3. Ждем, пока парсеры завершатся. Позволяем прервать ожидание.
		select {
		case <-parsersDoneChan:
			fmt.Printf("%s[INFO] Все парсеры (%d) завершили свою работу.%s\n", ColorBlue, len(parsers), ColorReset)
		case <-interruptChan:
			fmt.Printf("\n%s[INFO] Обнаружен сигнал остановки во время работы парсеров. Ожидаем их завершения...%s\n", ColorYellow, ColorReset)
			<-parsersDoneChan // Все равно дожидаемся завершения, чтобы не оставлять "висячих" процессов
			fmt.Printf("%s[INFO] Парсеры завершили работу. Выходим из цикла.%s\n", ColorBlue, ColorReset)
			keepRunning = false
			continue // Переходим к следующей итерации (которая будет последней)
		}

		// 4. Вычисляем оставшееся время и запускаем обратный отсчет
		remainingDuration := time.Until(deadline)
		if remainingDuration < 0 {
			fmt.Printf("%s[WARN] Парсеры работали дольше выделенного времени (%v). Немедленный перезапуск.%s\n", ColorYellow, totalWaitDuration, ColorReset)
			continue // Сразу переходим к следующей итерации
		}

		// Блок с таймером на ОСТАВШЕЕСЯ время
		timer := time.NewTimer(remainingDuration)
		stopCountdownChan := make(chan struct{})
		var wgCountdown sync.WaitGroup

		wgCountdown.Add(1)
		go func() {
			defer wgCountdown.Done()
			ticker := time.NewTicker(33 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-stopCountdownChan:
					return
				case <-ticker.C:
					remaining := time.Until(deadline)
					if remaining <= 0 {
						fmt.Printf("\r%s[INFO] До повторного запуска: 0m 0.000s%s ", ColorBlue, ColorReset)
						return
					}
					minutes := int(remaining.Minutes())
					seconds := remaining.Seconds() - float64(minutes*60)
					fmt.Printf("\r%s[INFO] До повторного запуска: %dm %.3fs%s ", ColorBlue, minutes, seconds, ColorReset)
				}
			}
		}()

		select {
		case <-timer.C:
			close(stopCountdownChan)
			wgCountdown.Wait()
			fmt.Printf("\r%s[INFO] До повторного запуска: 0m 0.000s%s \n", ColorBlue, ColorReset)
		case <-interruptChan:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			close(stopCountdownChan)
			wgCountdown.Wait()
			fmt.Println()
			fmt.Printf("%s[INFO] Завершаем цикл...%s\n", ColorBlue, ColorReset)
			keepRunning = false
		}
	}

	fmt.Printf("\n%s[INFO] Цикл парсинга завершен. Возврат в главное меню.%s\n", ColorBlue, ColorReset)
	time.Sleep(2 * time.Second)
}
