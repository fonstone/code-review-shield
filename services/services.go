// Package services 核心业务领域层统一门面。
// 暴露全部领域服务实例给handlers层；handler禁止写业务逻辑，业务全部下沉到本门面。
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"code-shield/models"
	"code-shield/services/dispatcher"
	"code-shield/services/engines"
	"code-shield/services/engines/registry"
	"code-shield/services/governance"
	"code-shield/services/invoker"
	"code-shield/services/queue"
	"code-shield/services/runner"
)

// Services 统一门面：持有全部领域服务
type Services struct {
	Cfg        *models.Config
	DB         *gorm.DB
	Dispatcher *dispatcher.ModelDispatcher
	Pool       *queue.WorkerPool
	Runner     *runner.Runner

	TaskTypes *TaskTypeService
	Tasks     *TaskService
	Repos     *RepoService
	Issues    *IssueService
	Reports   *ReportService
	Debug     *DebugService
	Schedules *ScheduleService
	Dashboard *DashboardService

	Feedback *governance.FeedbackStore
	Taxonomy *governance.Taxonomy
}

// NewServices 初始化全部领域服务（DB、调度器、队列、流水线、插件扫描）
func NewServices(cfg *models.Config) (*Services, error) {
	db, err := models.InitDB(&cfg.Database)
	if err != nil {
		return nil, err
	}
	if err := models.AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	// 调度器：注册全部LLM资源
	disp := dispatcher.NewModelDispatcher(30 * time.Minute)
	for _, res := range cfg.LLM.Resources {
		disp.AddResource(res.ID, res.Driver, res.Model, res.Concurrent, 1)
	}

	// 队列池（handler由runner接管）
	pool := queue.NewWorkerPool(cfg.Scanner.WorkerCount, cfg.Scanner.MaxQueueSize, nil)

	// 流水线协调器
	r := runner.NewRunner(db, cfg, disp, pool)

	s := &Services{
		Cfg:        cfg,
		DB:         db,
		Dispatcher: disp,
		Pool:       pool,
		Runner:     r,
		Feedback:   r.Feedback,
		Taxonomy:   r.Taxonomy,
	}
	s.TaskTypes = &TaskTypeService{svc: s}
	s.Tasks = &TaskService{svc: s}
	s.Repos = &RepoService{svc: s}
	s.Issues = &IssueService{svc: s}
	s.Reports = &ReportService{svc: s}
	s.Debug = &DebugService{svc: s}
	s.Schedules = &ScheduleService{svc: s, items: map[uint]*Schedule{}}
	s.Dashboard = &DashboardService{svc: s}

	// 启动时扫描tasks目录，同步插件入库
	if err := s.TaskTypes.SyncFromDisk(); err != nil {
		log.Printf("[WARN] 任务插件同步失败: %v", err)
	}
	return s, nil
}

// Start 启动队列消费与定时任务
func (s *Services) Start() {
	s.Runner.Start()
	s.Schedules.Start()
	log.Printf("[INFO] 服务已启动：%d 个LLM资源，%d 个Worker", len(s.Cfg.LLM.Resources), s.Cfg.Scanner.WorkerCount)
}

// Stop 优雅停止
func (s *Services) Stop() {
	s.Schedules.Stop()
	s.Runner.Stop()
}

// ==================== TaskTypeService 任务类型插件管理 ====================

// TaskTypeService 任务类型插件CRUD + 磁盘同步
type TaskTypeService struct {
	svc *Services
}

// pluginMeta tasks/<name>/meta.json Schema
type pluginMeta struct {
	Name            string          `json:"name"`
	DisplayName     string          `json:"display_name"`
	Description     string          `json:"description"`
	EngineMode      string          `json:"engine_mode"`
	EngineConfig    json.RawMessage `json:"engine_config"`
	AIBackend       string          `json:"ai_backend"`
	TargetScope     string          `json:"target_scope"`
	NotifyThreshold int             `json:"notify_threshold"`
	Timeout         int             `json:"timeout"`
	IsActive        *bool           `json:"is_active"`
	GovernanceMode  string          `json:"governance_mode"`
	IsCampaign      bool            `json:"is_campaign"`
}

// TasksDir 插件目录
func (s *TaskTypeService) TasksDir() string {
	return filepath.Join(s.svc.Cfg.Storage.Root, "tasks")
}

// SyncFromDisk 扫描tasks目录meta.json，校验并同步入库task_types
func (s *TaskTypeService) SyncFromDisk() error {
	dir := s.TasksDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取tasks目录失败: %w", err)
	}
	synced := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(dir, e.Name(), "meta.json")
		raw, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta pluginMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			log.Printf("[WARN] 插件 %s meta.json解析失败: %v", e.Name(), err)
			continue
		}
		if meta.Name == "" {
			meta.Name = e.Name()
		}
		if err := s.validateMeta(&meta); err != nil {
			log.Printf("[WARN] 插件 %s 校验失败: %v", meta.Name, err)
			continue
		}
		analysisPrompt := readFileOrEmpty(filepath.Join(dir, e.Name(), "analysis_prompt.md"))
		synthesisPrompt := readFileOrEmpty(filepath.Join(dir, e.Name(), "synthesis_prompt.md"))

		var existing models.TaskType
		err = s.svc.DB.Where("name = ?", meta.Name).First(&existing).Error
		active := true
		if meta.IsActive != nil {
			active = *meta.IsActive
		}
		if err == gorm.ErrRecordNotFound {
			tt := models.TaskType{
				Name:            meta.Name,
				DisplayName:     meta.DisplayName,
				Description:     meta.Description,
				EngineMode:      meta.EngineMode,
				EngineConfig:    datatypes.JSON(meta.EngineConfig),
				AIBackend:       meta.AIBackend,
				IsActive:        active,
				IsCampaign:      meta.IsCampaign,
				GovernanceMode:  meta.GovernanceMode,
				NotifyThreshold: meta.NotifyThreshold,
				TimeoutSeconds:  meta.Timeout,
				TargetScope:     meta.TargetScope,
				AnalysisPrompt:  analysisPrompt,
				SynthesisPrompt: synthesisPrompt,
			}
			if err := s.svc.DB.Create(&tt).Error; err != nil {
				return err
			}
			synced++
		} else if err == nil {
			// 已存在：同步配置（不覆盖用户手动改的激活状态）
			updates := map[string]any{
				"display_name":     meta.DisplayName,
				"description":      meta.Description,
				"engine_mode":      meta.EngineMode,
				"engine_config":    datatypes.JSON(meta.EngineConfig),
				"ai_backend":       meta.AIBackend,
				"governance_mode":  meta.GovernanceMode,
				"notify_threshold": meta.NotifyThreshold,
				"timeout_seconds":  meta.Timeout,
				"target_scope":     meta.TargetScope,
				"analysis_prompt":  analysisPrompt,
				"synthesis_prompt": synthesisPrompt,
				"updated_at":       time.Now(),
			}
			if err := s.svc.DB.Model(&models.TaskType{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	if synced > 0 {
		log.Printf("[INFO] 插件同步完成：新增 %d 个任务类型", synced)
	}
	return nil
}

func (s *TaskTypeService) validateMeta(m *pluginMeta) error {
	if m.Name == "" {
		return fmt.Errorf("name不能为空")
	}
	valid := map[string]bool{
		"single": true, "chunked": true, "chunked_fast": true,
		"debate_full": true, "debate_selective": true,
	}
	if !valid[m.EngineMode] {
		return fmt.Errorf("engine_mode非法: %s", m.EngineMode)
	}
	return nil
}

// List 任务类型列表
func (s *TaskTypeService) List(activeOnly bool) ([]models.TaskType, error) {
	var out []models.TaskType
	q := s.svc.DB.Order("is_campaign DESC, id ASC")
	if activeOnly {
		q = q.Where("is_active = ?", true)
	}
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// Get 单个任务类型
func (s *TaskTypeService) Get(id uint) (*models.TaskType, error) {
	var tt models.TaskType
	if err := s.svc.DB.First(&tt, id).Error; err != nil {
		return nil, err
	}
	return &tt, nil
}

// CreateReq 创建插件请求
type CreateReq struct {
	Name            string          `json:"name"`
	DisplayName     string          `json:"display_name"`
	Description     string          `json:"description"`
	EngineMode      string          `json:"engine_mode"`
	EngineConfig    json.RawMessage `json:"engine_config"`
	AIBackend       string          `json:"ai_backend"`
	TargetScope     string          `json:"target_scope"`
	NotifyThreshold int             `json:"notify_threshold"`
	Timeout         int             `json:"timeout"`
	GovernanceMode  string          `json:"governance_mode"`
	IsCampaign      bool            `json:"is_campaign"`
	AnalysisPrompt  string          `json:"analysis_prompt"`
	SynthesisPrompt string          `json:"synthesis_prompt"`
}

// Create 创建任务插件（同时落盘tasks/<name>/目录）
func (s *TaskTypeService) Create(req CreateReq) (*models.TaskType, error) {
	if req.Name == "" || req.DisplayName == "" {
		return nil, fmt.Errorf("name与display_name不能为空")
	}
	meta := pluginMeta{
		Name: req.Name, DisplayName: req.DisplayName, Description: req.Description,
		EngineMode: req.EngineMode, EngineConfig: req.EngineConfig, AIBackend: req.AIBackend,
		TargetScope: req.TargetScope, NotifyThreshold: req.NotifyThreshold, Timeout: req.Timeout,
		GovernanceMode: req.GovernanceMode, IsCampaign: req.IsCampaign,
	}
	if err := s.validateMeta(&meta); err != nil {
		return nil, err
	}
	// 落盘插件目录
	if err := s.writePluginFiles(req); err != nil {
		return nil, err
	}
	tt := models.TaskType{
		Name:            req.Name,
		DisplayName:     req.DisplayName,
		Description:     req.Description,
		EngineMode:      req.EngineMode,
		EngineConfig:    datatypes.JSON(req.EngineConfig),
		AIBackend:       req.AIBackend,
		TargetScope:     req.TargetScope,
		NotifyThreshold: req.NotifyThreshold,
		TimeoutSeconds:  req.Timeout,
		GovernanceMode:  req.GovernanceMode,
		IsCampaign:      req.IsCampaign,
		IsActive:        true,
		AnalysisPrompt:  req.AnalysisPrompt,
		SynthesisPrompt: req.SynthesisPrompt,
	}
	if err := s.svc.DB.Create(&tt).Error; err != nil {
		return nil, fmt.Errorf("插件名已存在或入库失败: %w", err)
	}
	return &tt, nil
}

// Update 更新插件配置
func (s *TaskTypeService) Update(id uint, req CreateReq) (*models.TaskType, error) {
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"display_name":     req.DisplayName,
		"description":      req.Description,
		"engine_mode":      req.EngineMode,
		"engine_config":    datatypes.JSON(req.EngineConfig),
		"ai_backend":       req.AIBackend,
		"target_scope":     req.TargetScope,
		"notify_threshold": req.NotifyThreshold,
		"timeout_seconds":  req.Timeout,
		"governance_mode":  req.GovernanceMode,
		"is_campaign":      req.IsCampaign,
		"updated_at":       time.Now(),
	}
	if req.AnalysisPrompt != "" {
		updates["analysis_prompt"] = req.AnalysisPrompt
	}
	if req.SynthesisPrompt != "" {
		updates["synthesis_prompt"] = req.SynthesisPrompt
	}
	if err := s.svc.DB.Model(&models.TaskType{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	// 同步落盘
	if err := s.writePluginFiles(req); err != nil {
		log.Printf("[WARN] 插件文件落盘失败: %v", err)
	}
	return s.Get(id)
}

// Delete 删除插件（同时删除tasks目录）
func (s *TaskTypeService) Delete(id uint) error {
	tt, err := s.Get(id)
	if err != nil {
		return err
	}
	var refs int64
	s.svc.DB.Model(&models.TaskReport{}).Where("task_type_id = ?", id).Count(&refs)
	if refs > 0 {
		return fmt.Errorf("该插件已有 %d 份历史报告，禁止删除（可改为停用）", refs)
	}
	if err := s.svc.DB.Delete(&models.TaskType{}, id).Error; err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(s.TasksDir(), tt.Name))
	return nil
}

// Activate 激活插件
func (s *TaskTypeService) Activate(id uint) error {
	return s.svc.DB.Model(&models.TaskType{}).Where("id = ?", id).Update("is_active", true).Error
}

// Deactivate 停用插件
func (s *TaskTypeService) Deactivate(id uint) error {
	return s.svc.DB.Model(&models.TaskType{}).Where("id = ?", id).Update("is_active", false).Error
}

func (s *TaskTypeService) writePluginFiles(req CreateReq) error {
	if req.Name == "" {
		return nil
	}
	dir := filepath.Join(s.TasksDir(), req.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	meta := pluginMeta{
		Name: req.Name, DisplayName: req.DisplayName, Description: req.Description,
		EngineMode: req.EngineMode, EngineConfig: req.EngineConfig, AIBackend: req.AIBackend,
		TargetScope: req.TargetScope, NotifyThreshold: req.NotifyThreshold, Timeout: req.Timeout,
		GovernanceMode: req.GovernanceMode, IsCampaign: req.IsCampaign,
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), b, 0o644); err != nil {
		return err
	}
	if req.AnalysisPrompt != "" {
		if err := os.WriteFile(filepath.Join(dir, "analysis_prompt.md"), []byte(req.AnalysisPrompt), 0o644); err != nil {
			return err
		}
	}
	if req.SynthesisPrompt != "" {
		if err := os.WriteFile(filepath.Join(dir, "synthesis_prompt.md"), []byte(req.SynthesisPrompt), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func readFileOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// ==================== RepoService 仓库管理 ====================

// RepoService 代码仓库管理
type RepoService struct {
	svc *Services
}

// List 仓库列表
func (s *RepoService) List() ([]models.Repository, error) {
	var out []models.Repository
	if err := s.svc.DB.Order("id DESC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// Create 创建仓库
func (s *RepoService) Create(name, url, branch string) (*models.Repository, error) {
	if url == "" {
		return nil, fmt.Errorf("仓库URL不能为空")
	}
	if name == "" {
		name = deriveRepoName(url)
	}
	var repo models.Repository
	err := s.svc.DB.Where("url = ?", url).First(&repo).Error
	if err == nil {
		return &repo, nil
	}
	repo = models.Repository{Name: name, URL: url, Branch: branch}
	if err := s.svc.DB.Create(&repo).Error; err != nil {
		return nil, err
	}
	return &repo, nil
}

// GetOrCreate 按ID或URL查找，不存在则创建
func (s *RepoService) GetOrCreate(repoID uint, name, url, branch string) (*models.Repository, error) {
	if repoID > 0 {
		var repo models.Repository
		if err := s.svc.DB.First(&repo, repoID).Error; err != nil {
			return nil, fmt.Errorf("仓库不存在: %w", err)
		}
		return &repo, nil
	}
	if url == "" {
		return nil, fmt.Errorf("必须提供repo_id或repo_url")
	}
	return s.Create(name, url, branch)
}

func deriveRepoName(url string) string {
	u := strings.TrimSuffix(strings.TrimSuffix(url, "/"), ".git")
	if i := strings.LastIndexAny(u, "/\\"); i >= 0 {
		return u[i+1:]
	}
	return u
}

// ==================== IssueService 缺陷追踪 ====================

// IssueService 缺陷追踪服务
type IssueService struct {
	svc *Services
}

// IssueQuery 缺陷查询
type IssueQuery struct {
	Page       int
	PageSize   int
	Severity   string
	Status     string
	DiffStatus string
	RepoID     uint
	TaskTypeID uint
	Keyword    string
}

// List 缺陷列表（分页）
func (s *IssueService) List(q IssueQuery) ([]models.AnalysisFinding, int64, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	dbq := s.svc.DB.Model(&models.AnalysisFinding{})
	if q.Severity != "" {
		dbq = dbq.Where("severity = ?", q.Severity)
	}
	if q.Status != "" {
		dbq = dbq.Where("status = ?", q.Status)
	}
	if q.DiffStatus != "" {
		dbq = dbq.Where("diff_status = ?", q.DiffStatus)
	}
	if q.RepoID > 0 {
		dbq = dbq.Where("repo_id = ?", q.RepoID)
	}
	if q.TaskTypeID > 0 {
		dbq = dbq.Where("task_type_id = ?", q.TaskTypeID)
	}
	if q.Keyword != "" {
		dbq = dbq.Where("title LIKE ? OR file_path LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%")
	}
	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []models.AnalysisFinding
	if err := dbq.Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// UpdateStatus 更新缺陷状态
func (s *IssueService) UpdateStatus(id uint, status string) error {
	return s.svc.DB.Model(&models.AnalysisFinding{}).Where("id = ?", id).Update("status", status).Error
}

// Suppress 标记误报（写入误报记忆库）
func (s *IssueService) Suppress(id uint, reason, operator string) error {
	var f models.AnalysisFinding
	if err := s.svc.DB.First(&f, id).Error; err != nil {
		return err
	}
	s.svc.Feedback.Suppress(f.L1Fingerprint, f.L2Fingerprint, reason, operator)
	return s.svc.DB.Model(&models.AnalysisFinding{}).Where("id = ?", id).
		Update("status", models.FindingSuppressed).Error
}

// ==================== ScheduleService 定时调度 ====================

// Schedule 定时扫描计划（内存态）
type Schedule struct {
	ID          uint       `json:"id"`
	RepoID      uint       `json:"repo_id"`
	TaskTypeID  uint       `json:"task_type_id"`
	IntervalMin int        `json:"interval_minutes"`
	SinceDays   int        `json:"since_days"`
	Enabled     bool       `json:"enabled"`
	LastRunAt   *time.Time `json:"last_run_at"`
	NextRunAt   time.Time  `json:"next_run_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ScheduleService 定时调度服务
type ScheduleService struct {
	svc    *Services
	mu     sync.Mutex
	seq    uint
	items  map[uint]*Schedule
	ticker *time.Ticker
	stopCh chan struct{}
}

// Add 新增计划
func (s *ScheduleService) Add(repoID, taskTypeID uint, intervalMin, sinceDays int) (*Schedule, error) {
	if repoID == 0 || taskTypeID == 0 {
		return nil, fmt.Errorf("repo_id与task_type_id必填")
	}
	if intervalMin < 5 {
		intervalMin = 5
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	sc := &Schedule{
		ID: s.seq, RepoID: repoID, TaskTypeID: taskTypeID, IntervalMin: intervalMin,
		SinceDays: sinceDays, Enabled: true, CreatedAt: time.Now(),
		NextRunAt: time.Now().Add(time.Duration(intervalMin) * time.Minute),
	}
	s.items[sc.ID] = sc
	return sc, nil
}

// List 计划列表
func (s *ScheduleService) List() []*Schedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Schedule, 0, len(s.items))
	for _, it := range s.items {
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Delete 删除计划
func (s *ScheduleService) Delete(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
}

// Start 启动调度循环
func (s *ScheduleService) Start() {
	s.stopCh = make(chan struct{})
	s.ticker = time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-s.stopCh:
				return
			case <-s.ticker.C:
				s.runDue()
			}
		}
	}()
}

// Stop 停止调度
func (s *ScheduleService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	if s.stopCh != nil {
		close(s.stopCh)
	}
}

func (s *ScheduleService) runDue() {
	s.mu.Lock()
	var due []*Schedule
	for _, it := range s.items {
		if it.Enabled && time.Now().After(it.NextRunAt) {
			due = append(due, it)
		}
	}
	s.mu.Unlock()
	for _, it := range due {
		_, err := s.svc.Tasks.Trigger(TriggerReq{RepoID: it.RepoID, TaskTypeID: it.TaskTypeID, SinceDays: it.SinceDays})
		if err != nil {
			log.Printf("[WARN] 定时任务触发失败(schedule=%d): %v", it.ID, err)
		}
		s.mu.Lock()
		now := time.Now()
		it.LastRunAt = &now
		it.NextRunAt = now.Add(time.Duration(it.IntervalMin) * time.Minute)
		s.mu.Unlock()
	}
}

// ==================== DashboardService 看板统计 ====================

// DashboardService 数据看板服务
type DashboardService struct {
	svc *Services
}

// Summary 全局指标
func (s *DashboardService) Summary() map[string]any {
	db := s.svc.DB
	var totalTasks, runningTasks, pendingFindings int64
	db.Model(&models.TaskReport{}).Count(&totalTasks)
	db.Model(&models.TaskReport{}).Where("status = ?", models.StatusAnalyzing).Count(&runningTasks)
	db.Model(&models.AnalysisFinding{}).Where("status = ? AND diff_status <> ?", models.FindingOpen, models.DiffResolved).Count(&pendingFindings)

	today := time.Now().Format("2006-01-02")
	var todayNew, todayFixed, todayReopened int64
	db.Model(&models.AnalysisFinding{}).Where("diff_status = ? AND DATE(created_at) = ?", models.DiffNew, today).Count(&todayNew)
	db.Model(&models.AnalysisFinding{}).Where("diff_status = ? AND DATE(updated_at) = ?", models.DiffResolved, today).Count(&todayFixed)
	db.Model(&models.AnalysisFinding{}).Where("diff_status = ? AND DATE(created_at) = ?", models.DiffReopened, today).Count(&todayReopened)

	activeSlots := 0
	for _, r := range s.svc.Dispatcher.Resources() {
		activeSlots += r.ActiveSlots
	}
	return map[string]any{
		"total_tasks":      totalTasks,
		"running_tasks":    runningTasks,
		"active_slots":     activeSlots,
		"pending_findings": pendingFindings,
		"today_new":        todayNew,
		"today_fixed":      todayFixed,
		"today_reopened":   todayReopened,
	}
}

// Trends 缺陷趋势（按天）
func (s *DashboardService) Trends(days int) []map[string]any {
	if days <= 0 {
		days = 14
	}
	out := make([]map[string]any, 0, days)
	db := s.svc.DB
	for i := days - 1; i >= 0; i-- {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		var newN, fixedN, reopenedN int64
		db.Model(&models.AnalysisFinding{}).Where("diff_status = ? AND DATE(created_at) = ?", models.DiffNew, day).Count(&newN)
		db.Model(&models.AnalysisFinding{}).Where("diff_status = ? AND DATE(updated_at) = ?", models.DiffResolved, day).Count(&fixedN)
		db.Model(&models.AnalysisFinding{}).Where("diff_status = ? AND DATE(created_at) = ?", models.DiffReopened, day).Count(&reopenedN)
		out = append(out, map[string]any{"date": day, "new": newN, "fixed": fixedN, "reopened": reopenedN})
	}
	return out
}

// RepoRank 仓库质量排行（缺陷密度）
func (s *DashboardService) RepoRank() []map[string]any {
	type row struct {
		RepoID   uint
		RepoName string
		Total    int64
		Critical int64
		High     int64
	}
	var rows []row
	s.svc.DB.Model(&models.AnalysisFinding{}).
		Select("analysis_findings.repo_id as repo_id, repositories.name as repo_name, COUNT(*) as total, "+
			"SUM(CASE WHEN severity='CRITICAL' THEN 1 ELSE 0 END) as critical, "+
			"SUM(CASE WHEN severity='HIGH' THEN 1 ELSE 0 END) as high").
		Joins("LEFT JOIN repositories ON repositories.id = analysis_findings.repo_id").
		Where("analysis_findings.status <> ?", models.FindingSuppressed).
		Group("analysis_findings.repo_id, repositories.name").
		Scan(&rows)
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var avg float64
		s.svc.DB.Model(&models.TaskReport{}).Where("repo_id = ? AND status = ?", r.RepoID, models.StatusSuccess).
			Select("COALESCE(AVG(score),0)").Scan(&avg)
		out = append(out, map[string]any{
			"repo_id": r.RepoID, "repo_name": r.RepoName, "total_findings": r.Total,
			"critical_count": r.Critical, "high_count": r.High, "score": int(avg),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i]["total_findings"].(int64) > out[j]["total_findings"].(int64)
	})
	return out
}

// ModelStats AI模型调用统计
func (s *DashboardService) ModelStats() []map[string]any {
	out := make([]map[string]any, 0)
	for _, r := range s.svc.Dispatcher.Resources() {
		failureRate := 0.0
		if r.TotalCalls > 0 {
			failureRate = float64(r.FailedCalls) / float64(r.TotalCalls)
		}
		out = append(out, map[string]any{
			"resource_id": r.ID, "model": r.Model, "total_calls": r.TotalCalls,
			"failed_calls": r.FailedCalls, "failure_rate": failureRate,
			"avg_duration_ms": r.AvgDurationMs(),
		})
	}
	return out
}

// ==================== DebugService 系统诊断 ====================

// DebugService 管理员运维服务
type DebugService struct {
	svc *Services
}

// Info 系统诊断大盘
func (s *DebugService) Info() map[string]any {
	resources := make([]map[string]any, 0)
	for _, r := range s.svc.Dispatcher.Resources() {
		resources = append(resources, map[string]any{
			"id": r.ID, "driver": r.Driver, "model": r.Model, "concurrent": r.Concurrent,
			"active_slots": r.ActiveSlots, "load_ratio": r.LoadRatio(),
			"total_calls": r.TotalCalls, "failed_calls": r.FailedCalls,
			"avg_duration_ms": r.AvgDurationMs(),
		})
	}
	stats := s.svc.Pool.Stats()
	return map[string]any{
		"resources": resources,
		"workers": map[string]any{
			"worker_count": stats.WorkerCount, "busy_workers": stats.BusyWorkers,
			"queue_capacity": stats.QueueCapacity, "queued": stats.Queued,
			"processed": stats.Processed, "failed": stats.Failed,
		},
		"running_tasks": s.svc.Runner.RunningTasks(),
	}
}

// ResetSlots 槽位校准
func (s *DebugService) ResetSlots() dispatcher.ResetResult {
	return s.svc.Dispatcher.ResetActiveSlots()
}

// Leases 租约透视
func (s *DebugService) Leases() []*dispatcher.LLMSlotLease {
	leases := s.svc.Dispatcher.Leases()
	sort.SliceStable(leases, func(i, j int) bool { return leases[i].AcquiredAt.After(leases[j].AcquiredAt) })
	return leases
}

// Queue 队列状态透视
func (s *DebugService) Queue() map[string]any {
	stats := s.svc.Pool.Stats()
	var items []models.QueueTask
	s.svc.DB.Order("id DESC").Limit(200).Find(&items)
	return map[string]any{
		"worker_count": stats.WorkerCount, "busy_workers": stats.BusyWorkers,
		"queue_capacity": stats.QueueCapacity, "queued": stats.Queued,
		"processed": stats.Processed, "failed": stats.Failed, "items": items,
	}
}

// ==================== 编译期接口断言 ====================

var _ = engines.TaskEngine(nil)
var _ = registry.NewEngine
var _ = invoker.CreateInvoker
var _ = context.Background

// ==================== TaskService 任务生命周期 ====================

// TaskService 扫描任务服务：触发/列表/详情/删除/取消/恢复
type TaskService struct {
	svc *Services
}

// TriggerReq 触发扫描请求
type TriggerReq struct {
	RepoID     uint   `json:"repo_id"`
	RepoName   string `json:"repo_name"`
	RepoURL    string `json:"repo_url"`
	Branch     string `json:"branch"`
	TaskTypeID uint   `json:"task_type_id"`
	SinceDays  int    `json:"since_days"`
	DiffBase   string `json:"diff_base"`
	Priority   int    `json:"priority"`
}

// Trigger 手动触发扫描任务：创建报告+入队
func (s *TaskService) Trigger(req TriggerReq) (*models.TaskReport, error) {
	tt, err := s.svc.TaskTypes.Get(req.TaskTypeID)
	if err != nil {
		return nil, fmt.Errorf("任务类型不存在: %w", err)
	}
	if !tt.IsActive {
		return nil, fmt.Errorf("任务类型已停用: %s", tt.DisplayName)
	}
	repo, err := s.svc.Repos.GetOrCreate(req.RepoID, req.RepoName, req.RepoURL, req.Branch)
	if err != nil {
		return nil, err
	}

	metrics, _ := json.Marshal(map[string]any{
		"since_days": req.SinceDays,
		"diff_base":  req.DiffBase,
	})
	report := models.TaskReport{
		RepoID:      repo.ID,
		TaskTypeID:  tt.ID,
		Status:      models.StatusPending,
		CloneStatus: models.ClonePending,
		Metrics:     metrics,
	}
	if err := s.svc.DB.Create(&report).Error; err != nil {
		return nil, err
	}
	if err := s.enqueue(report.ID, req.Priority); err != nil {
		return nil, err
	}
	return &report, nil
}

// enqueue 创建队列记录并尝试入队（队列满则留在pending由cron补投）
func (s *TaskService) enqueue(reportID uint, priority int) error {
	qt := models.QueueTask{
		TaskReportID: reportID,
		Status:       models.QueuePending,
		Priority:     priority,
		MaxAttempts:  s.svc.Cfg.Scanner.Analysis.MaxRetries,
	}
	if qt.MaxAttempts <= 0 {
		qt.MaxAttempts = 3
	}
	if err := s.svc.DB.Create(&qt).Error; err != nil {
		return err
	}
	if err := s.svc.Pool.Enqueue(queue.Job{QueueTaskID: qt.ID, TaskReportID: reportID, Priority: priority, Attempt: 0}); err != nil {
		log.Printf("[WARN] 队列已满，任务#%d等待补投", reportID)
	}
	return nil
}

// DrainQueue 补投pending队列任务（cron调用）
func (s *TaskService) DrainQueue() int {
	var pending []models.QueueTask
	now := time.Now()
	s.svc.DB.Where("status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", models.QueuePending, now).
		Order("priority DESC, id ASC").Limit(50).Find(&pending)
	n := 0
	for _, qt := range pending {
		if err := s.svc.Pool.Enqueue(queue.Job{
			QueueTaskID: qt.ID, TaskReportID: qt.TaskReportID, Priority: qt.Priority, Attempt: qt.Attempts,
		}); err == nil {
			n++
		}
	}
	return n
}

// TaskQuery 任务列表查询
type TaskQuery struct {
	Page       int
	PageSize   int
	Status     string
	RepoID     uint
	TaskTypeID uint
	StartTime  string
	EndTime    string
}

// List 任务列表（分页+多状态筛选）
func (s *TaskService) List(q TaskQuery) ([]models.TaskReport, int64, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	dbq := s.svc.DB.Model(&models.TaskReport{})
	if q.Status != "" {
		dbq = dbq.Where("status = ?", q.Status)
	}
	if q.RepoID > 0 {
		dbq = dbq.Where("repo_id = ?", q.RepoID)
	}
	if q.TaskTypeID > 0 {
		dbq = dbq.Where("task_type_id = ?", q.TaskTypeID)
	}
	if q.StartTime != "" {
		dbq = dbq.Where("created_at >= ?", q.StartTime)
	}
	if q.EndTime != "" {
		dbq = dbq.Where("created_at <= ?", q.EndTime)
	}
	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []models.TaskReport
	if err := dbq.Preload("Repo").Preload("TaskType").
		Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Get 任务详情（含缺陷列表）
func (s *TaskService) Get(id uint) (*models.TaskReport, error) {
	var report models.TaskReport
	if err := s.svc.DB.Preload("Repo").Preload("TaskType").First(&report, id).Error; err != nil {
		return nil, err
	}
	var findings []models.AnalysisFinding
	s.svc.DB.Where("task_report_id = ?", id).
		Order("CASE severity WHEN 'CRITICAL' THEN 0 WHEN 'HIGH' THEN 1 WHEN 'MEDIUM' THEN 2 ELSE 3 END, id ASC").
		Find(&findings)
	if findings == nil {
		findings = []models.AnalysisFinding{}
	}
	report.Findings = findings
	return &report, nil
}

// Delete 删除任务报告（running任务禁止删除）
func (s *TaskService) Delete(id uint) error {
	var report models.TaskReport
	if err := s.svc.DB.First(&report, id).Error; err != nil {
		return err
	}
	if report.Status == models.StatusAnalyzing {
		return fmt.Errorf("任务正在执行中，禁止删除")
	}
	return s.svc.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_report_id = ?", id).Delete(&models.AnalysisFinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_report_id = ?", id).Delete(&models.QueueTask{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.TaskReport{}, id).Error; err != nil {
			return err
		}
		if report.ReportPath != "" {
			_ = os.Remove(report.ReportPath)
		}
		return nil
	})
}

// CancelRunning 批量取消正在执行任务
func (s *TaskService) CancelRunning() int {
	return s.svc.Runner.CancelRunning()
}

// Resume 失败分片恢复续跑
func (s *TaskService) Resume(id uint) error {
	return s.svc.Runner.Resume(context.Background(), id)
}

// ==================== ReportService 报告导出 ====================

// ReportService 报告导出/预览服务
type ReportService struct {
	svc *Services
}

// Export 导出报告：markdown / json / excel
func (s *ReportService) Export(id uint, format string) (data []byte, contentType, filename string, err error) {
	report, err := s.svc.Tasks.Get(id)
	if err != nil {
		return nil, "", "", err
	}
	switch format {
	case "json":
		payload := map[string]any{
			"report":   report,
			"findings": report.Findings,
		}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, "", "", err
		}
		return b, "application/json; charset=utf-8", fmt.Sprintf("code-shield-report-%d.json", id), nil
	case "excel":
		b, err := s.exportExcel(report)
		if err != nil {
			return nil, "", "", err
		}
		return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			fmt.Sprintf("code-shield-report-%d.xlsx", id), nil
	default: // markdown
		return []byte(s.renderMarkdown(report)), "text/markdown; charset=utf-8",
			fmt.Sprintf("code-shield-report-%d.md", id), nil
	}
}

// Preview 报告预览（Markdown）
func (s *ReportService) Preview(id uint) (string, error) {
	report, err := s.svc.Tasks.Get(id)
	if err != nil {
		return "", err
	}
	return s.renderMarkdown(report), nil
}

func (s *ReportService) renderMarkdown(report *models.TaskReport) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Code-Shield 扫描报告 #%d\n\n", report.ID))
	sb.WriteString("## 基本信息\n\n")
	sb.WriteString(fmt.Sprintf("- 仓库: %s\n", repoName(report)))
	sb.WriteString(fmt.Sprintf("- 任务类型: %s\n", taskTypeName(report)))
	sb.WriteString(fmt.Sprintf("- 状态: %s\n", report.Status))
	sb.WriteString(fmt.Sprintf("- 评分: %d\n", report.Score))
	sb.WriteString(fmt.Sprintf("- 分片: %d/%d 成功, %d 失败\n", report.SuccessChunks, report.TotalChunks, report.FailedChunks))
	sb.WriteString(fmt.Sprintf("- 生成时间: %s\n\n", report.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString("## 审计总结\n\n")
	if report.AISummary != "" {
		sb.WriteString(report.AISummary)
	} else {
		sb.WriteString("（无AI总结）")
	}
	sb.WriteString("\n\n## 缺陷清单\n\n")
	if len(report.Findings) == 0 {
		sb.WriteString("未发现缺陷。\n")
		return sb.String()
	}
	sb.WriteString("| 严重度 | 标题 | 文件 | 行号 | 分类 | Diff | 状态 |\n")
	sb.WriteString("|--------|------|------|------|------|------|------|\n")
	for _, f := range report.Findings {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
			f.Severity, escapePipe(f.Title), f.FilePath, f.LineNumber, f.Category, f.DiffStatus, f.Status))
	}
	return sb.String()
}

func escapePipe(s string) string { return strings.ReplaceAll(s, "|", "\\|") }

func repoName(r *models.TaskReport) string {
	if r.Repo != nil {
		return r.Repo.Name
	}
	return fmt.Sprintf("repo-%d", r.RepoID)
}

func taskTypeName(r *models.TaskReport) string {
	if r.TaskType != nil {
		return r.TaskType.DisplayName
	}
	return fmt.Sprintf("type-%d", r.TaskTypeID)
}

// exportExcel 生成Excel报告（xlsx）
func (s *ReportService) exportExcel(report *models.TaskReport) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// Sheet1: 概览
	overview := "概览"
	f.SetSheetName("Sheet1", overview)
	rows := [][]any{
		{"报告ID", report.ID},
		{"仓库", repoName(report)},
		{"任务类型", taskTypeName(report)},
		{"状态", report.Status},
		{"评分", report.Score},
		{"总分片", report.TotalChunks},
		{"成功分片", report.SuccessChunks},
		{"失败分片", report.FailedChunks},
		{"生成时间", report.CreatedAt.Format("2006-01-02 15:04:05")},
	}
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		_ = f.SetSheetRow(overview, cell, &row)
	}

	// Sheet2: 缺陷清单
	sheet2 := "缺陷清单"
	_, _ = f.NewSheet(sheet2)
	header := []any{"ID", "严重度", "标题", "文件路径", "行号", "分类", "Diff状态", "状态", "置信度", "详情", "建议"}
	_ = f.SetSheetRow(sheet2, "A1", &header)
	for i, fd := range report.Findings {
		row := []any{fd.ID, fd.Severity, fd.Title, fd.FilePath, fd.LineNumber, fd.Category,
			fd.DiffStatus, fd.Status, fd.Confidence, fd.Detail, fd.Suggestion}
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetSheetRow(sheet2, cell, &row)
	}
	// 列宽
	_ = f.SetColWidth(sheet2, "A", "A", 8)
	_ = f.SetColWidth(sheet2, "B", "B", 10)
	_ = f.SetColWidth(sheet2, "C", "C", 40)
	_ = f.SetColWidth(sheet2, "D", "D", 40)
	_ = f.SetColWidth(sheet2, "J", "K", 50)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}