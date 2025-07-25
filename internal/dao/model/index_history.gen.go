// Code generated by gorm.io/gen. DO NOT EDIT.
// Code generated by gorm.io/gen. DO NOT EDIT.
// Code generated by gorm.io/gen. DO NOT EDIT.

package model

import (
	"time"
)

const TableNameIndexHistory = "index_history"

// IndexHistory mapped from table <index_history>
type IndexHistory struct {
	ID                int32      `gorm:"column:id;primaryKey;autoIncrement:true;comment:Unique identifier for the index task history record" json:"id"`        // Unique identifier for the index task history record
	SyncID            int32      `gorm:"column:sync_id;not null;comment:ID of the associated synchronization history record" json:"sync_id"`                   // ID of the associated synchronization history record
	CodebaseID        int32      `gorm:"column:codebase_id;not null;comment:ID of the associated project repository" json:"codebase_id"`                       // ID of the associated project repository
	CodebasePath      string     `gorm:"column:codebase_path;not null;comment:FilePaths of the project repository" json:"codebase_path"`                            // FilePaths of the project repository
	CodebaseName      string     `gorm:"column:codebase_name;not null;comment:name of the project repository" json:"codebase_name"`                            // name of the project repository
	TotalFileCount    *int32     `gorm:"column:total_file_count;comment:Total number of files" json:"total_file_count"`                                        // Total number of files
	TotalSuccessCount *int32     `gorm:"column:total_success_count;comment:Total success number of files" json:"total_success_count"`                          // Total success number of files
	TotalFailCount    *int32     `gorm:"column:total_fail_count;comment:Total fail number of files" json:"total_fail_count"`                                   // Total fail number of files
	TotalIgnoreCount  *int32     `gorm:"column:total_ignore_count;comment:Total ignore number of files" json:"total_ignore_count"`                             // Total ignore number of files
	TaskType          string     `gorm:"column:task_type;not null;comment:SubmitTask type: vector, relation" json:"task_type"`                                       // SubmitTask type: vector, relation
	Status            string     `gorm:"column:status;not null;comment:SubmitTask status: pending, running, success, failed" json:"status"`                          // SubmitTask status: pending, running, success, failed
	Progress          *float64   `gorm:"column:progress;comment:SubmitTask progress (floating point number between 0 and 1)" json:"progress"`                        // SubmitTask progress (floating point number between 0 and 1)
	ErrorMessage      *string    `gorm:"column:error_message;comment:Error message if the task failed" json:"error_message"`                                   // Error message if the task failed
	StartTime         *time.Time `gorm:"column:start_time;comment:SubmitTask start time" json:"start_time"`                                                          // SubmitTask start time
	EndTime           *time.Time `gorm:"column:end_time;comment:SubmitTask end time" json:"end_time"`                                                                // SubmitTask end time
	CreatedAt         time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP;comment:Time when the record was created" json:"created_at"`      // Time when the record was created
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP;comment:Time when the record was last updated" json:"updated_at"` // Time when the record was last updated
}

// TableName IndexHistory's table name
func (*IndexHistory) TableName() string {
	return TableNameIndexHistory
}
