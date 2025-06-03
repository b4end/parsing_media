package main

import (
	"bufio"
	"fmt"
	"os"
	"parsing_media/parsers"
	. "parsing_media/utils"
	"sync"
	"time"
)

func main() {
	for true {
		var num string
		fmt.Printf("%sMENU BAR%s\n! - Exit\n0 - Run all\n1 - RIA\n2 - Gazeta\n3 - Lenta\n4 - Vesti\n5 - Kommersant\n6 - MK\n7 - Fontanka\n8 - Smotrim\n9 - Banki\n10 - DumaTV\nOption: ", ColorYellow, ColorReset)
		fmt.Scan(&num)

		switch num {
		case "!":
			return

		case "0":
			interruptChan := make(chan struct{})

			go func() {
				reader := bufio.NewReader(os.Stdin)
				_, err := reader.ReadString('\n')
				if err != nil {
				}
				select {
				case <-interruptChan:
				default:
					close(interruptChan)
				}
			}()

			keepRunning := true
			for keepRunning {
				var wg sync.WaitGroup

				parserJobs := []struct {
					name string
					fn   func()
				}{
					{"RIA", parsers.RiaMain},
					{"Gazeta", parsers.GazetaMain},
					{"Lenta", parsers.LentaMain},
					{"Vesti", parsers.VestiMain},
					{"Kommersant", parsers.KommersMain},
					{"MK", parsers.MKMain},
					{"Fontanka", parsers.FontankaMain},
					{"Smotrim", parsers.SmotrimMain},
					{"Banki", parsers.BankiMain},
					{"DumaTV", parsers.DumaTVMain},
				}

				fmt.Printf("\n%s[INFO] Запуск всех парсеров%s\n\n", ColorBlue, ColorReset)

				for _, job := range parserJobs {
					wg.Add(1)
					go func(parserName string, parserFunc func()) {
						defer wg.Done()
						parserFunc()
					}(job.name, job.fn)
				}

				wg.Wait()

				fmt.Printf("\n%s[INFO] Все парсеры завершили свою работу.%s\n", ColorBlue, ColorReset)

				select {
				case <-interruptChan:
					fmt.Printf("%s[INFO] Обнаружено нажатие Enter. Остановка цикла...%s\n", ColorBlue, ColorReset)
					keepRunning = false
					continue
				default:
				}

				if !keepRunning {
					break
				}

				fmt.Printf("%s[INFO] Ожидание 3 минуты перед следующим запуском. Нажмите Enter для остановки.%s\n", ColorBlue, ColorReset)

				timer := time.NewTimer(3 * time.Minute)
				select {
				case <-interruptChan:
					fmt.Printf("%s[INFO] Обнаружено нажатие Enter. Остановка цикла...%s\n", ColorBlue, ColorReset)
					keepRunning = false
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
				case <-timer.C:
					fmt.Printf("%s[INFO] Таймаут истек. Перезапуск парсеров...%s\n\n", ColorBlue, ColorReset)
				}
			}
			fmt.Printf("\n%s[INFO] Цикл парсинга завершен.%s\n", ColorBlue, ColorReset)

		case "1":
			parsers.RiaMain()

		case "2":
			parsers.GazetaMain()

		case "3":
			parsers.LentaMain()

		case "4":
			parsers.VestiMain()

		case "5":
			parsers.KommersMain()

		case "6":
			parsers.MKMain()

		case "7":
			parsers.FontankaMain()

		case "8":
			parsers.SmotrimMain()

		case "9":
			parsers.BankiMain()

		case "10":
			parsers.DumaTVMain()

		}
	}
}
