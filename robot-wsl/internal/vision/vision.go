package vision

import (
	"image"
	"log"
	"sync"
	"time"

	"robot-client/internal/state"

	ort "github.com/yalue/onnxruntime_go"
	"gocv.io/x/gocv"
)

type Vision struct {
	mu            sync.RWMutex
	state         *state.SharedState
	session       *ort.AdvancedSession
	opts          *ort.SessionOptions
	inputTensor   *ort.Tensor[float32]
	outputTensor  *ort.Tensor[float32]
	modelPath     string
	confidenceThr float64
	inputSize     int
}

func NewVision(st *state.SharedState, modelPath string, confidenceThr float64) *Vision {
	ort.SetSharedLibraryPath("onnxruntime.dll")

	opts, err := ort.NewSessionOptions()
	if err != nil {
		log.Fatalf("Ошибка создания SessionOptions: %v", err)
	}

	// Создаём входной и выходной тензоры один раз
	inputTensor, err := ort.NewEmptyTensor[float32]([]int64{1, 3, 640, 640})
	if err != nil {
		log.Fatalf("Ошибка создания входного тензора: %v", err)
	}

	outputTensor, err := ort.NewEmptyTensor[float32]([]int64{1, 84, 8400})
	if err != nil {
		inputTensor.Destroy()
		log.Fatalf("Ошибка создания выходного тензора: %v", err)
	}

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"images"},
		[]string{"output0"},
		[]ort.Value{inputTensor},
		[]ort.Value{outputTensor},
		opts,
	)
	if err != nil {
		opts.Destroy()
		inputTensor.Destroy()
		outputTensor.Destroy()
		log.Fatalf("Ошибка создания сессии ONNX Runtime: %v", err)
	}

	return &Vision{
		state:         st,
		session:       session,
		opts:          opts,
		inputTensor:   inputTensor,
		outputTensor:  outputTensor,
		modelPath:     modelPath,
		confidenceThr: confidenceThr,
		inputSize:     640,
	}
}

func (v *Vision) Start(videoCapture *gocv.VideoCapture) {
	go v.processLoop(videoCapture)
}

func (v *Vision) processLoop(videoCapture *gocv.VideoCapture) {
	frame := gocv.NewMat()
	defer frame.Close()

	for {
		if ok := videoCapture.Read(&frame); !ok || frame.Empty() {
			time.Sleep(20 * time.Millisecond)
			continue
		}

		detections := v.ProcessFrame(&frame)
		v.state.UpdateDetectedObjects(detections)
	}
}

func (v *Vision) ProcessFrame(frame *gocv.Mat) []state.DetectedObject {
	if err := v.preprocess(frame); err != nil {
		log.Printf("[Vision] Препроцессинг error: %v", err)
		return nil
	}

	err := v.session.Run()
	if err != nil {
		log.Printf("[Vision] Инференс error: %v", err)
		return nil
	}

	data := v.outputTensor.GetData()
	log.Printf("[Vision] Первые 8 значений после Run: %.4f, %.4f, %.4f, %.4f, %.4f, %.4f, %.4f, %.4f",
		data[0], data[1], data[2], data[3], data[4], data[5], data[6], data[7])

	return v.postprocess(v.outputTensor, frame)
}

func (v *Vision) preprocess(frame *gocv.Mat) error {
	resized := gocv.NewMat()
	defer resized.Close()

	// Меняем размер кадра до нужного для модели
	gocv.Resize(*frame, &resized, image.Pt(v.inputSize, v.inputSize), 0, 0, gocv.InterpolationLinear)

	// Создаём blob (нормализация + преобразование в CHW)
	blob := gocv.BlobFromImage(resized, 1.0/255.0, image.Pt(v.inputSize, v.inputSize),
		gocv.NewScalar(0, 0, 0, 0), true, false)
	defer blob.Close()

	// Получаем данные в виде []float32
	data, err := blob.DataPtrFloat32()
	if err != nil {
		return err
	}

	// Копируем данные в заранее созданный входной тензор
	copy(v.inputTensor.GetData(), data)

	return nil
}

func (v *Vision) postprocess(output *ort.Tensor[float32], frame *gocv.Mat) []state.DetectedObject {
	data := output.GetData()
	if len(data) == 0 {
		return nil
	}

	numClasses := 80
	stride := numClasses + 4
	rows := len(data) / stride

	var detections []state.DetectedObject

	for i := 0; i < rows; i++ {
		base := i * stride

		// Пробуем трактовать как пиксельные x1, y1, x2, y2
		x1 := data[base+0]
		y1 := data[base+1]
		x2 := data[base+2]
		y2 := data[base+3]

		maxScore := float32(0)
		classID := -1
		for j := 0; j < numClasses; j++ {
			if data[base+4+j] > maxScore {
				maxScore = data[base+4+j]
				classID = j
			}
		}

		if maxScore < float32(v.confidenceThr) || classID == -1 {
			continue
		}

		if maxScore > 1.0 {
			maxScore = 1.0
		}

		// Нормализуем в [-1, 1]
		cx := ((x1+x2)/2/float32(frame.Cols()))*2 - 1
		cy := ((y1+y2)/2/float32(frame.Rows()))*2 - 1

		cx = clamp(cx, -1, 1)
		cy = clamp(cy, -1, 1)

		width := (x2 - x1) / float32(frame.Cols())
		height := (y2 - y1) / float32(frame.Rows())

		detections = append(detections, state.DetectedObject{
			Class:      getClassName(classID),
			Confidence: maxScore,
			X:          cx,
			Y:          cy,
			Width:      width,
			Height:     height,
			Timestamp:  time.Now(),
		})
	}

	return detections
}

func getClassName(classID int) string {
	classes := []string{
		"person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck", "boat",
		"traffic light", "fire hydrant", "stop sign", "parking meter", "bench", "bird", "cat",
		"dog", "horse", "sheep", "cow", "elephant", "bear", "zebra", "giraffe", "backpack",
		"umbrella", "handbag", "tie", "suitcase", "frisbee", "skis", "snowboard", "sports ball",
		"kite", "baseball bat", "baseball glove", "skateboard", "surfboard", "tennis racket",
		"bottle", "wine glass", "cup", "fork", "knife", "spoon", "bowl", "banana", "apple",
		"sandwich", "orange", "broccoli", "carrot", "hot dog", "pizza", "donut", "cake",
		"chair", "couch", "potted plant", "bed", "dining table", "toilet", "tv", "laptop",
		"mouse", "remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink",
		"refrigerator", "book", "clock", "vase", "scissors", "teddy bear", "hair drier", "toothbrush",
	}

	if classID >= 0 && classID < len(classes) {
		return classes[classID]
	}
	return "unknown"
}

func clamp(val, minVal, maxVal float32) float32 {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func (v *Vision) Close() {
	if v.inputTensor != nil {
		v.inputTensor.Destroy()
	}
	if v.outputTensor != nil {
		v.outputTensor.Destroy()
	}
	if v.session != nil {
		v.session.Destroy()
	}
	if v.opts != nil {
		v.opts.Destroy()
	}
}
