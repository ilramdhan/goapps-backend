package orchestrator

import (
	"github.com/mutugading/goapps-backend/pkg/costcalc"
)

// ChunkSpec is the unit of work dispatched to the worker pool. It carries the
// product IDs in this chunk + job-level context. Serialized to JSON on the
// finance.cost.chunk RabbitMQ queue.
type ChunkSpec struct {
	JobID       int64                    `json:"job_id"`
	JobCode     string                   `json:"job_code"`
	ChunkID     int64                    `json:"chunk_id"`
	ChunkNumber int                      `json:"chunk_number"`
	WaveNo      int                      `json:"wave_no"`
	Period      string                   `json:"period"`
	CalcType    costcalc.CalculationType `json:"calculation_type"`
	ProductIDs  []int64                  `json:"product_ids"`
	Actor       string                   `json:"actor"`
}

// PackedWave groups chunks belonging to the same wave for sequential dispatch.
// The orchestrator runs waves in order; chunks within a wave can run in parallel.
type PackedWave struct {
	Number int
	Chunks []ChunkSpec
}

const defaultChunkSize = 50

// PackChunks turns a wave plan into ordered chunk groups.
// Each wave's products are split into chunks of at most chunkSize products.
// Chunk numbering is sequential across the entire job (1-based) so that
// cal_job_chunk.cjc_chunk_number is globally unique within the job (matches
// the UNIQUE(cjc_job_id, cjc_chunk_number) constraint).
func PackChunks(plan *costcalc.WavePlan, chunkSize, maxChunkSize int) []PackedWave {
	if plan == nil {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if maxChunkSize > 0 && chunkSize > maxChunkSize {
		chunkSize = maxChunkSize
	}
	out := make([]PackedWave, 0, len(plan.Waves))
	globalChunkNumber := 0
	for _, wave := range plan.Waves {
		pw := PackedWave{Number: wave.Number}
		for i := 0; i < len(wave.Products); i += chunkSize {
			end := i + chunkSize
			if end > len(wave.Products) {
				end = len(wave.Products)
			}
			globalChunkNumber++
			pw.Chunks = append(pw.Chunks, ChunkSpec{
				ChunkNumber: globalChunkNumber,
				WaveNo:      wave.Number,
				ProductIDs:  append([]int64{}, wave.Products[i:end]...),
			})
		}
		out = append(out, pw)
	}
	return out
}

// FillJobContext applies job-level context to every chunk in every wave.
// Convenient for the coordinator (S8c.5) which packs first then fills in.
func FillJobContext(waves []PackedWave, jobID int64, jobCode, period, actor string, calcType costcalc.CalculationType) {
	for i := range waves {
		for j := range waves[i].Chunks {
			waves[i].Chunks[j].JobID = jobID
			waves[i].Chunks[j].JobCode = jobCode
			waves[i].Chunks[j].Period = period
			waves[i].Chunks[j].Actor = actor
			waves[i].Chunks[j].CalcType = calcType
		}
	}
}
