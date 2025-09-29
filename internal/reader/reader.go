package reader

import (
	"errors"
	"io"
)

// Modification описывает одно изменение в файле.
type Modification struct {
	Offset int64  // Смещение от начала файла, куда применяются новые данные.
	Data   []byte // Новые данные.
}

// ModifiedReader предоставляет view для io.ReadSeeker с примененными изменениями.
// Он реализует интерфейс io.ReadSeeker.
type ModifiedReader struct {
	original     io.ReadSeeker
	originalSize int64

	mods        []Modification // Срез всех примененных изменений.
	currentPos  int64          // Текущая позиция для Read и Seek.
	virtualSize int64          // Общий размер "виртуального" файла после изменений.
}

// NewModifiedReader создает новый ModifiedReader, оборачивая оригинальный io.ReadSeeker.
func NewModifiedReader(original io.ReadSeeker) (*ModifiedReader, error) {
	// Получаем и кэшируем оригинальный размер файла.
	originalSize, err := original.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	// Возвращаем курсор в начало на всякий случай.
	if _, err := original.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	return &ModifiedReader{
		original:     original,
		originalSize: originalSize,
		mods:         make([]Modification, 0), // Инициализируем пустой срез изменений.
		currentPos:   0,
		virtualSize:  originalSize, // Изначально виртуальный размер равен оригинальному.
	}, nil
}

// Modify добавляет или заменяет часть данных в "виртуальном" файле.
// Этот метод можно вызывать многократно.
func (mr *ModifiedReader) Modify(offset int64, data []byte) {
	mod := Modification{
		Offset: offset,
		Data:   data,
	}
	mr.mods = append(mr.mods, mod)

	// Пересчитываем виртуальный размер.
	// Если изменение выходит за пределы текущего виртуального размера,
	// то файл "увеличивается".
	modEnd := offset + int64(len(data))
	if modEnd > mr.virtualSize {
		mr.virtualSize = modEnd
	}
}

// Seek реализует io.Seeker для перемещения по "виртуальному" файлу.
func (mr *ModifiedReader) Seek(offset int64, whence int) (int64, error) {
	var newPos int64

	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = mr.currentPos + offset
	case io.SeekEnd:
		newPos = mr.virtualSize + offset
	default:
		return 0, errors.New("seek: invalid whence")
	}

	if newPos < 0 {
		return 0, errors.New("seek: invalid offset")
	}

	mr.currentPos = newPos
	return newPos, nil
}

// Read реализует io.Reader для чтения из "виртуального" файла.
// Логика обрабатывает чтение из оригинального источника и измененных областей.
func (mr *ModifiedReader) Read(p []byte) (n int, err error) {
	// Если курсор находится в конце или за пределами файла, возвращаем EOF.
	if mr.currentPos >= mr.virtualSize {
		return 0, io.EOF
	}

	// Ограничиваем чтение размером виртуального файла.
	bytesToRead := len(p)
	if mr.currentPos+int64(bytesToRead) > mr.virtualSize {
		bytesToRead = int(mr.virtualSize - mr.currentPos)
	}

	totalRead := 0
	// Цикл для заполнения буфера p, так как он может пересекать
	// несколько областей (оригинал, изменение 1, изменение 2 и т.д.).
	for totalRead < bytesToRead {
		pos := mr.currentPos

		// Ищем последнее ("last write wins") изменение, которое покрывает текущую позицию.
		// Идем по срезу в обратном порядке, чтобы найти самое новое изменение для этой позиции.
		var coveringMod *Modification
		for i := len(mr.mods) - 1; i >= 0; i-- {
			mod := &mr.mods[i]
			if pos >= mod.Offset && pos < (mod.Offset+int64(len(mod.Data))) {
				coveringMod = mod
				break
			}
		}

		if coveringMod != nil {
			// --- Случай 1: Позиция находится внутри области изменения ---
			modOffset := pos - coveringMod.Offset
			modBytesLeft := int64(len(coveringMod.Data)) - modOffset
			chunkSize := min(int64(bytesToRead-totalRead), modBytesLeft)

			copied := copy(p[totalRead:], coveringMod.Data[modOffset:modOffset+chunkSize])
			totalRead += copied
			mr.currentPos += int64(copied)

		} else {
			// --- Случай 2: Позиция находится в области оригинальных данных ---

			// Находим, где начинается следующее изменение после текущей позиции.
			nextModOffset := mr.virtualSize
			for i := range mr.mods {
				if mr.mods[i].Offset > pos && mr.mods[i].Offset < nextModOffset {
					nextModOffset = mr.mods[i].Offset
				}
			}

			// Определяем, сколько можно прочитать до следующего изменения.
			bytesUntilNextMod := nextModOffset - pos
			chunkSize := min(int64(bytesToRead-totalRead), bytesUntilNextMod)

			// Если текущая позиция за пределами оригинального файла, это "дыра",
			// созданная изменением, которое расширило файл. Читаем нули.
			if pos >= mr.originalSize {
				// Заполняем нулями
				for i := 0; i < int(chunkSize); i++ {
					p[totalRead+i] = 0
				}
				readBytes := int(chunkSize)
				totalRead += readBytes
				mr.currentPos += int64(readBytes)
			} else {
				// Читаем из оригинального файла.
				if _, err := mr.original.Seek(pos, io.SeekStart); err != nil {
					return totalRead, err
				}

				// Не позволяем читать за пределами оригинального файла.
				if pos+chunkSize > mr.originalSize {
					chunkSize = mr.originalSize - pos
				}
				if chunkSize == 0 {
					break // Достигли конца оригинальной части файла
				}

				readBytes, readErr := mr.original.Read(p[totalRead : totalRead+int(chunkSize)])
				totalRead += readBytes
				mr.currentPos += int64(readBytes)

				if readErr != nil && readErr != io.EOF {
					return totalRead, readErr
				}
			}
		}
	}

	// Если ничего не прочитали, но должны были, проверяем EOF еще раз.
	if totalRead == 0 && mr.currentPos >= mr.virtualSize {
		return 0, io.EOF
	}

	return totalRead, nil
}

// Вспомогательная функция для нахождения минимума из двух int64.
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
