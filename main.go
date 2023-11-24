package main

import (
	"fmt"
	"gocv.io/x/gocv"
	"sync"
)

const l = 10
const Zmin = 0
const Zmax = 255

// HDR структура для создания HDR изображений
type HDR struct {
	Images []gocv.Mat // LDR Изображения
	N      int        // Количество изображений
	Times  []float32  //
	Row    int        // Размер изображений по горизонтали
	Col    int        // Размер изображений по вертикали
	L      float32    // Коэффициент гладкости

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
	// Здесь должен быть код для выравнивания изображений
	// alignMTB := gocv.NewAlignMTB()...
}

func main() {
	filenames := []string{"uploads/image0.jpg", "uploads/image1.jpg", "uploads/image2.jpg"}
	exposureTimes := []float32{1.0, 0.5, 0.25}
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

}
