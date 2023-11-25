// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.22.0

package queries

import (
	"github.com/jackc/pgx/v5/pgtype"
)

type Config struct {
	SingleRow                bool
	DefaultPriceCpuCoreHour  float64
	DefaultPriceMemoryGbHour float64
	PriceCpuCoreHour         pgtype.Float8
	PriceMemoryGigabyteHour  pgtype.Float8
}

type CostPodHourly struct {
	Timestamp          pgtype.Timestamptz
	PodUid             pgtype.UUID
	Namespace          string
	PodName            string
	NodeName           string
	CreatedAt          pgtype.Timestamptz
	StartedAt          pgtype.Timestamptz
	DeletedAt          pgtype.Timestamptz
	RequestMemoryBytes float64
	RequestCpuCores    float64
	Labels             []byte
	Annotations        []byte
	ControllerUid      pgtype.UUID
	ControllerKind     string
	ControllerName     string
	CpuCoresAvg        float64
	CpuCoresMax        float64
	MemoryBytesAvg     float64
	MemoryBytesMax     float64
	PodHours           int32
	MemoryCost         int32
	CpuCost            int32
}

type CostWorkloadDaily struct {
	Timestamp             pgtype.Interval
	Namespace             string
	ControllerKind        string
	ControllerName        string
	MemoryBytesAvg        int32
	MemoryBytesMax        interface{}
	RequestMemoryBytesAvg int32
	CpuCoresAvg           int32
	CpuCoresMax           interface{}
	RequestCpuCoresAvg    int32
	PodHours              int64
	MemoryCost            int64
	CpuCost               int64
	TotalCost             int64
}

type Job struct {
	JobUid         pgtype.UUID
	Namespace      string
	Name           string
	ControllerKind string
	ControllerName string
	ControllerUid  pgtype.UUID
	CreatedAt      pgtype.Timestamptz
	DeletedAt      pgtype.Timestamptz
	Labels         []byte
	Annotations    []byte
}

type Pod struct {
	PodUid             pgtype.UUID
	Namespace          string
	Name               string
	NodeName           string
	CreatedAt          pgtype.Timestamptz
	StartedAt          pgtype.Timestamptz
	DeletedAt          pgtype.Timestamptz
	RequestCpuCores    float64
	RequestMemoryBytes float64
	ControllerKind     string
	ControllerName     string
	ControllerUid      pgtype.UUID
	Labels             []byte
	Annotations        []byte
}

type PodController struct {
	PodUid         pgtype.UUID
	Name           string
	Namespace      string
	ControllerUid  pgtype.UUID
	ControllerKind string
	ControllerName string
}

type PodUsageHourly struct {
	PodUid                   pgtype.UUID
	Timestamp                pgtype.Timestamptz
	MemoryBytesMax           float64
	MemoryBytesMin           float64
	MemoryBytesTotal         float64
	MemoryBytesTotalReadings int32
	MemoryBytesAvg           float64
	CpuCoresMax              float64
	CpuCoresMin              float64
	CpuCoresTotal            float64
	CpuCoresTotalReadings    int32
	CpuCoresAvg              float64
}

type ReplicaSet struct {
	ReplicaSetUid  pgtype.UUID
	Namespace      string
	Name           string
	ControllerKind string
	ControllerName string
	ControllerUid  pgtype.UUID
	CreatedAt      pgtype.Timestamptz
	DeletedAt      pgtype.Timestamptz
	Labels         []byte
	Annotations    []byte
}
