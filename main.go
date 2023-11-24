package main

import (
	"fmt"
	"gocv.io/x/gocv"
	"sync"
)

const l = 10 // Коэффициент гладкости

// HDR структура для создания HDR изображений
type HDR struct {
	Images []gocv.Mat // LDR Изображения
	N      int        // Количество изображений
	Times  []float32  //
	Row    int        // Размер изображений по горизонтали
	Col    int        // Размер изображений по вертикали
	L      float32    // Коэффициент гладкости
	W      []float32  // Массив для хранения весов
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

}
