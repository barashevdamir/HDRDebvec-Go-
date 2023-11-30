package main

import (
	"fmt"
	"gocv.io/x/gocv"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"image/color"
	"log"
	"math"
	"sync"
)

const l = 10 // Коэффициент гладкости
const n = 256

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
	CRFB         []float64   // Массив для хранения функции отклика для синего канала
	CRFG         []float64   // Массив для хранения функции отклика для зеленого канала
	CRFR         []float64   // Массив для хранения функции отклика для красного канала
	lEB          []float64   // Массив для хранения логарифмической освещенности в местоположении пикселя i для синего канала
	lEG          []float64   // Массив для хранения логарифмической освещенности в местоположении пикселя i для зеленого канала
	lER          []float64   // Массив для хранения логарифмической освещенности в местоположении пикселя i для красного канала
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
				hdr.FlattenImage[i][channelIndex][y*cols+x] = pixel
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
	// Общее количество пикселей в изображении.
	pixels := hdr.Row * hdr.Col
	// Количество образцов для выборки.
	samples := int(math.Ceil(float64(255*2/(hdr.N-1))) * 2)
	if samples > pixels {
		samples = pixels
	}
	// Шаг выборки для равномерного распределения выборки по всему изображению.
	step := int(math.Ceil(float64(pixels) / float64(samples)))
	hdr.Indices = make([]int, 0, samples)
	for i := 0; i < pixels; i += step {
		hdr.Indices = append(hdr.Indices, i)
	}
	// Если последний индекс не был добавлен и еще есть место, добавим его вручную
	if len(hdr.Indices) < samples && hdr.Indices[len(hdr.Indices)-1] != pixels-1 {
		hdr.Indices = append(hdr.Indices, pixels-1)
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

// CRFsolve решает систему уравнений для получения функции отклика камеры и логарифмических значений освещенности.
func (hdr *HDR) CRFsolve(Z [][]uint8) (CRF []float64, logE []float64) {
	s1, s2 := len(Z), len(Z[0])
	U := mat.NewDense(s1*s2+n+1, n+s1, nil)
	V := mat.NewDense(U.RawMatrix().Rows, 1, nil)
	k := 0
	for i := 0; i < s1; i++ {
		for j := 0; j < s2; j++ {
			wij := float64(hdr.W[Z[i][j]])
			U.Set(k, int(Z[i][j]), wij)
			U.Set(k, n+i, -wij)
			V.Set(k, 0, wij*hdr.Bij[i][j])
			k++
		}
	}
	U.Set(k, 129, 0)
	k++
	for i := 1; i < n-2; i++ {
		U.Set(k, i, float64(hdr.L*hdr.W[i+1]))
		U.Set(k, i+1, float64(-2*hdr.L*hdr.W[i+1]))
		U.Set(k, i+2, float64(-hdr.L*hdr.W[i+1]))
		k++
	}
	var M mat.Dense
	M.Solve(U, V)
	CRF = mat.Col(nil, 0, M.Slice(0, n, 0, 1))
	logE = mat.Col(nil, 0, M.Slice(n, M.RawMatrix().Rows, 0, 1))

	return CRF, logE
}

// plotResponseCurves отображает графики функций отклика для каждого цветового канала.
func (hdr *HDR) plotResponseCurves() {
	p := plot.New()
	p.Title.Text = "Кривые отклика"
	p.X.Label.Text = "Значение пикселя"
	p.Y.Label.Text = "log(CRF)"

	// Создание линий для каждого цветового канала
	plotCRF := func(crf []float64, clr color.Color) {
		pts := make(plotter.XYs, 256)
		for i := range pts {
			pts[i].X = float64(i)
			pts[i].Y = math.Exp(crf[i])
		}

		line, err := plotter.NewLine(pts)
		if err != nil {
			log.Fatalf("Ошибка при создании линии: %v", err)
		}
		line.Color = clr

		p.Add(line)
	}

	plotCRF(hdr.CRFB, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	plotCRF(hdr.CRFG, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	plotCRF(hdr.CRFR, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	// Сохранение графика в файл
	if err := p.Save(5*vg.Inch, 5*vg.Inch, "results/curvesCRF.png"); err != nil {
		log.Fatalf("Ошибка при сохранении графика: %v", err)
	}

}

// Process вызывает предыдущие методы для построения функции отклика и подготовки данных для восстановления карты освещенности HDR.
func (hdr *HDR) Process() {
	// Вызов методов для подготовки данных
	hdr.weightingFunction()
	hdr.samplingValues()
	// Выполнение CRFsolve для каждого цветового канала
	hdr.CRFB, hdr.lEB = hdr.CRFsolve(hdr.ZB)
	hdr.CRFG, hdr.lEG = hdr.CRFsolve(hdr.ZG)
	hdr.CRFR, hdr.lER = hdr.CRFsolve(hdr.ZR)
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
	hdr.Process()
}
