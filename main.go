package main

import (
	"fmt"
	"gocv.io/x/gocv"
	"math"
	"sync"
)

const l = 10 // Коэффициент гладкости

// HDR структура для создания HDR изображений
type HDR struct {
	Images       []gocv.Mat  // LDR Изображения
	N            int         // Количество изображений
	Times        []float32   // Времена экспозиции
	Row          int         // Размер изображений по горизонтали
	Col          int         // Размер изображений по вертикали
	L            float32     // Коэффициент гладкости
	W            []float32   // Массив для хранения весов
	Indices      []int       // Индексы выбранных пикселей
	FlattenImage [][][]uint8 // Одномерное представление изображений
	ZB           [][]uint8   // Массив для хранения синего канала
	ZG           [][]uint8   // Массив для хранения зеленого канала
	ZR           [][]uint8   // Массив для хранения красного канала
	Bij          [][]float64 // Массив для хранения логарифмических времен экспозиции
}

// newHDR создает новый экземпляр HDR
func NewHDR(filenames []string, exposureTimes []float32) *HDR {
	hdr := &HDR{
		Times: exposureTimes,
		L:     l,
	}

	hdr.loadImages(filenames)
	hdr.alignImages()

	return hdr
}

// loadImages загружает изображения асинхронно
func (hdr *HDR) loadImages(filenames []string) {
	var wg sync.WaitGroup
	hdr.Images = make([]gocv.Mat, len(filenames))

	for i, filename := range filenames {
		wg.Add(1)
		go func(i int, filename string) {
			defer wg.Done()
			img := gocv.IMRead(filename, gocv.IMReadColor)
			hdr.Images[i] = img
		}(i, filename)
	}

	wg.Wait()

	for _, img := range hdr.Images {
		if img.Empty() {
			fmt.Println("Одно из изображений пустое")
			// Обработка ошибки загрузки изображения
		}
	}

	// Установка размеров изображения
	hdr.N = len(hdr.Images)
	if hdr.N > 0 {
		hdr.Row = hdr.Images[0].Rows()
		hdr.Col = hdr.Images[0].Cols()
	}
}

// alignImages выравнивает изображения
func (hdr *HDR) alignImages() {
	// Создание объекта для выравнивания
	alignMTB := gocv.NewAlignMTB()
	defer func(alignMTB *gocv.AlignMTB) {
		err := alignMTB.Close()
		if err != nil {

		}
	}(&alignMTB)
	// Подготовка слайса из gocv.Mat для выравнивания
	mats := make([]gocv.Mat, len(hdr.Images))
	for i, img := range hdr.Images {
		mats[i] = img.Clone()
		defer func(mat *gocv.Mat) {
			err := mat.Close()
			if err != nil {

			}
		}(&mats[i])
	}
	// Замена исходных изображений выровненными
	for i := range hdr.Images {
		err := hdr.Images[i].Close()
		if err != nil {
			return
		} // Освобождение ресурсов исходного изображения
		hdr.Images[i] = mats[i].Clone() // Копирование выровненного изображения обратно в hdr.Images
	}
}

// weightingFunction создает весовую функцию, которая придает больший вес хорошо экспонированным и меньший вес плохо экспонированным пикселям
func (hdr *HDR) weightingFunction() {
	Zmin := 0
	Zmax := 255
	Zmid := (Zmax + Zmin) / 2
	// Инициализация среза для весов
	hdr.W = make([]float32, Zmax-Zmin+1)

	for z := Zmin; z <= Zmax; z++ {
		if z <= Zmid {
			hdr.W[z] = float32(z - Zmin + 1)
		} else {
			hdr.W[z] = float32(z - Zmin + 1)
		}
	}
}

func (hdr *HDR) flattenChannel(channelIndex int, wg *sync.WaitGroup) {
	defer wg.Done()

	for i := 0; i < hdr.N; i++ {
		img := hdr.Images[i]
		rows := img.Rows()
		cols := img.Cols()

		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				pixel := img.GetUCharAt(y, x*img.Channels()+channelIndex)
				hdr.FlattenImage[i][channelIndex][y*cols+x] = pixel // Обратите внимание на правильное заполнение
			}
		}
	}
}

func generateIndices(samples int) []int {
	indices := make([]int, samples)
	for i := 0; i < samples; i++ {
		indices[i] = i
	}
	return indices
}

// samplingValues выбирает пиксели из каждого изображения для определения функции отклика камеры.
func (hdr *HDR) samplingValues() {
	pixels := hdr.Row * hdr.Col                             // Общее количество пикселей в изображении.
	samples := int(math.Ceil(float64(255*2/(hdr.N-1))) * 2) // Количество образцов для выборки.
	step := pixels / samples                                // Шаг выборки для равномерного распределения выборки по всему изображению.
	hdr.Indices = make([]int, 0, samples)
	for i := 0; i < pixels; i += step {
		hdr.Indices = append(hdr.Indices, i)
	}
	hdr.FlattenImage = make([][][]uint8, hdr.N)
	for i := range hdr.FlattenImage {
		hdr.FlattenImage[i] = make([][]uint8, 3) // Для каждого цветового канала
		for j := range hdr.FlattenImage[i] {
			hdr.FlattenImage[i][j] = make([]uint8, hdr.Row*hdr.Col) // Инициализация среза для каждого канала
		}
	}

	var wg sync.WaitGroup
	for channelIndex := 0; channelIndex < 3; channelIndex++ {
		wg.Add(1)
		go hdr.flattenChannel(channelIndex, &wg)
	}
	wg.Wait()
	hdr.ZB = make([][]uint8, samples)
	hdr.ZG = make([][]uint8, samples)
	hdr.ZR = make([][]uint8, samples)

	for i := range hdr.ZB {
		hdr.ZB[i] = make([]uint8, hdr.N)
		hdr.ZG[i] = make([]uint8, hdr.N)
		hdr.ZR[i] = make([]uint8, hdr.N)
	}

	// Получение выбранных значений пикселей
	for k := 0; k < hdr.N; k++ {
		for i, index := range hdr.Indices {
			hdr.ZB[i][k] = hdr.FlattenImage[k][0][index]
			hdr.ZG[i][k] = hdr.FlattenImage[k][1][index]
			hdr.ZR[i][k] = hdr.FlattenImage[k][2][index]
		}
	}

	ind := generateIndices(samples)

	newZB := make([][]uint8, len(ind))
	newZG := make([][]uint8, len(ind))
	newZR := make([][]uint8, len(ind))

	for i, idx := range ind {
		newZB[i] = make([]uint8, hdr.N)
		newZG[i] = make([]uint8, hdr.N)
		newZR[i] = make([]uint8, hdr.N)

		for j := 0; j < hdr.N; j++ {
			newZB[i][j] = hdr.ZB[idx][j]
			newZG[i][j] = hdr.ZG[idx][j]
			newZR[i][j] = hdr.ZR[idx][j]
		}
	}

	hdr.ZB = newZB
	hdr.ZG = newZG
	hdr.ZR = newZR

	// Массив для хранения логарифмических времен экспозиции
	hdr.Bij = make([][]float64, hdr.Row*hdr.Col)
	for i := range hdr.Bij {
		hdr.Bij[i] = make([]float64, len(hdr.Times))
		for j := range hdr.Times {
			hdr.Bij[i][j] = math.Log(float64(hdr.Times[j]))
		}
	}
}

func main() {
	filenames := []string{"uploads/image0.jpg", "uploads/image1.jpg", "uploads/image2.jpg"}
	exposureTimes := []float32{1 / 6.0, 1.3, 5.0}
	hdr := NewHDR(filenames, exposureTimes)
	// Вывод информации об изображениях
	fmt.Println("Количество изображений:", hdr.N)
	fmt.Println("Размеры изображений (строки x столбцы):", hdr.Row, "x", hdr.Col)
	// Освобождаем ресурсы
	for _, img := range hdr.Images {
		err := img.Close()
		if err != nil {
			return
		}
	}
	hdr.weightingFunction()
	// Вывод значений весовой функции (можно выводить часть массива для проверки)
	fmt.Println("Пример значений весовой функции:", hdr.W[125:135])
	hdr.samplingValues()

}
