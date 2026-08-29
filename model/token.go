package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Token struct {
	Id                  int                            `json:"id"`
	UserId              int                            `json:"user_id" gorm:"index"`
	Key                 string                         `json:"key" gorm:"type:varchar(128);uniqueIndex"`
	Status              int                            `json:"status" gorm:"default:1"`
	Name                string                         `json:"name" gorm:"index" `
	CreatedTime         int64                          `json:"created_time" gorm:"bigint"`
	AccessedTime        int64                          `json:"accessed_time" gorm:"bigint"`
	ExpiredTime         int64                          `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota         int                            `json:"remain_quota" gorm:"default:0"`
	UnlimitedQuota      bool                           `json:"unlimited_quota"`
	ModelLimitsEnabled  bool                           `json:"model_limits_enabled"`
	ModelLimits         string                         `json:"model_limits" gorm:"type:text"`
	ModelMapping        string                         `json:"model_mapping" gorm:"type:text"`
	AllowIps            *string                        `json:"allow_ips" gorm:"default:''"`
	UsedQuota           int                            `json:"used_quota" gorm:"default:0"` // used quota
	Group               string                         `json:"group" gorm:"default:auto"`
	GroupRef            *ratio_setting.PricingGroupRef `json:"group_ref,omitempty" gorm:"-"`
	AutoGroupCandidates PricingGroupCandidates         `json:"auto_group_candidates" gorm:"type:text"`
	CrossGroupRetry     bool                           `json:"-"` // kept for rolling rollback compatibility
	DeletedAt           gorm.DeletedAt                 `gorm:"index"`
}

type PricingGroupCandidates string

var ErrTokenRoutingMigrationPending = errors.New(
	"Auto group selection is temporarily unavailable while the database migration is pending; select all groups or try again shortly",
)

var ErrTokenModelMappingMigrationPending = errors.New(
	"Model mapping is temporarily unavailable while the database migration is pending; try again shortly",
)

var ErrUserTokenLimitReached = errors.New("user token limit reached")

const tokenModelMappingColumnRefreshInterval = 5 * time.Second

var tokenModelMappingColumnCache struct {
	sync.RWMutex
	db        *gorm.DB
	checkedAt time.Time
	available bool
}

// hasTokenModelMappingColumn caches the schema capability for regular reads.
// A missing column is rechecked periodically so a non-master started before a
// migration begins using it without a restart. A successful master migration
// calls refreshTokenModelMappingColumnCache immediately.
func hasTokenModelMappingColumn() bool {
	db := DB
	now := time.Now()
	tokenModelMappingColumnCache.RLock()
	cacheFresh := tokenModelMappingColumnCache.db == db &&
		(tokenModelMappingColumnCache.available || now.Sub(tokenModelMappingColumnCache.checkedAt) < tokenModelMappingColumnRefreshInterval)
	available := tokenModelMappingColumnCache.available
	tokenModelMappingColumnCache.RUnlock()
	if cacheFresh {
		return available
	}

	tokenModelMappingColumnCache.Lock()
	defer tokenModelMappingColumnCache.Unlock()
	if tokenModelMappingColumnCache.db == db &&
		(tokenModelMappingColumnCache.available || now.Sub(tokenModelMappingColumnCache.checkedAt) < tokenModelMappingColumnRefreshInterval) {
		return tokenModelMappingColumnCache.available
	}
	tokenModelMappingColumnCache.db = db
	tokenModelMappingColumnCache.checkedAt = now
	tokenModelMappingColumnCache.available = db.Migrator().HasColumn(&Token{}, "model_mapping")
	return tokenModelMappingColumnCache.available
}

func refreshTokenModelMappingColumnCache() {
	tokenModelMappingColumnCache.Lock()
	tokenModelMappingColumnCache.db = DB
	tokenModelMappingColumnCache.checkedAt = time.Now()
	tokenModelMappingColumnCache.available = DB.Migrator().HasColumn(&Token{}, "model_mapping")
	tokenModelMappingColumnCache.Unlock()
}

func NewPricingGroupCandidates(groups []string) PricingGroupCandidates {
	normalized := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		normalized = append(normalized, group)
	}
	return PricingGroupCandidates(strings.Join(normalized, ","))
}

func (c PricingGroupCandidates) Values() []string {
	if strings.TrimSpace(string(c)) == "" {
		return []string{}
	}
	normalized := string(NewPricingGroupCandidates(strings.Split(string(c), ",")))
	if normalized == "" {
		return []string{}
	}
	return strings.Split(normalized, ",")
}

func (c PricingGroupCandidates) MarshalJSON() ([]byte, error) {
	return common.Marshal(c.Values())
}

func (c *PricingGroupCandidates) UnmarshalJSON(data []byte) error {
	var groups []string
	if err := common.Unmarshal(data, &groups); err != nil {
		return err
	}
	*c = NewPricingGroupCandidates(groups)
	return nil
}

func (token *Token) NormalizeRouting() {
	if strings.TrimSpace(token.Group) == "" {
		token.Group = "auto"
	} else {
		token.Group = ratio_setting.PricingGroupKey(token.Group)
	}
	candidates := token.AutoGroupCandidates.Values()
	for i, candidate := range candidates {
		candidates[i] = ratio_setting.PricingGroupKey(candidate)
	}
	token.AutoGroupCandidates = NewPricingGroupCandidates(candidates)
	if token.Group != "auto" {
		token.AutoGroupCandidates = ""
	}
}

func (token *Token) BeforeCreate(_ *gorm.DB) error {
	token.NormalizeRouting()
	return nil
}

func (token *Token) AfterFind(_ *gorm.DB) error {
	token.NormalizeRouting()
	return nil
}

func migrateLegacyTokenGroupsToAuto() error {
	return DB.Unscoped().
		Model(&Token{}).
		Where(commonGroupCol+" IS NULL OR "+commonGroupCol+" = ?", "").
		UpdateColumn("group", "auto").Error
}

func (token *Token) Clean() {
	token.Key = ""
}

func MaskTokenKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return strings.Repeat("*", len(key))
	}
	if len(key) <= 8 {
		return key[:2] + "****" + key[len(key)-2:]
	}
	return key[:4] + "**********" + key[len(key)-4:]
}

func (token *Token) GetFullKey() string {
	return token.Key
}

func (token *Token) GetMaskedKey() string {
	return MaskTokenKey(token.Key)
}

func (token *Token) GetIpLimits() []string {
	// delete empty spaces
	//split with \n
	ipLimits := make([]string, 0)
	if token.AllowIps == nil {
		return ipLimits
	}
	cleanIps := strings.ReplaceAll(*token.AllowIps, " ", "")
	if cleanIps == "" {
		return ipLimits
	}
	ips := strings.Split(cleanIps, "\n")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		ip = strings.ReplaceAll(ip, ",", "")
		if ip != "" {
			ipLimits = append(ipLimits, ip)
		}
	}
	return ipLimits
}

func GetAllUserTokens(userId int, startIdx int, num int) ([]*Token, error) {
	var tokens []*Token
	var err error
	err = tokenReadDB(DB).Where("user_id = ?", userId).Order("id desc").Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, err
}

func tokenReadDB(db *gorm.DB) *gorm.DB {
	if hasTokenModelMappingColumn() {
		return db
	}
	return db.Omit("model_mapping")
}

// sanitizeLikePattern 校验并清洗用户输入的 LIKE 搜索模式。
// 规则：
//  1. 转义 ! 和 _（使用 ! 作为 ESCAPE 字符，兼容 MySQL/PostgreSQL/SQLite）
//  2. 连续的 % 合并为单个 %
//  3. 最多允许 2 个 %
//  4. 含 % 时（模糊搜索），去掉 % 后关键词长度必须 >= 2
//  5. 不含 % 时按精确匹配
func sanitizeLikePattern(input string) (string, error) {
	// 1. 先转义 ESCAPE 字符 ! 自身，再转义 _
	//    使用 ! 而非 \ 作为 ESCAPE 字符，避免 MySQL 中反斜杠的字符串转义问题
	input = strings.ReplaceAll(input, "!", "!!")
	input = strings.ReplaceAll(input, `_`, `!_`)

	if err := validateLikePattern(input); err != nil {
		return "", err
	}

	// 5. 无 % 时，精确全匹配
	return input, nil
}

func validateLikePattern(input string) error {
	// 1. 连续的 % 直接拒绝
	if strings.Contains(input, "%%") {
		return errors.New("搜索模式中不允许包含连续的 % 通配符")
	}

	// 2. 统计 % 数量，不得超过 2
	count := strings.Count(input, "%")
	if count > 2 {
		return errors.New("搜索模式中最多允许包含 2 个 % 通配符")
	}

	// 3. 含 % 时，去掉 % 后关键词长度必须 >= 2
	if count > 0 {
		stripped := strings.ReplaceAll(input, "%", "")
		if len(stripped) < 2 {
			return errors.New("使用模糊搜索时，关键词长度至少为 2 个字符")
		}
	}

	return nil
}

const searchHardLimit = 100

func SearchUserTokens(userId int, keyword string, token string, offset int, limit int) (tokens []*Token, total int64, err error) {
	// model 层强制截断
	if limit <= 0 || limit > searchHardLimit {
		limit = searchHardLimit
	}
	if offset < 0 {
		offset = 0
	}

	if token != "" {
		token = strings.TrimPrefix(token, "sk-")
	}

	// 超量用户（令牌数超过上限）只允许精确搜索，禁止模糊搜索
	maxTokens := operation_setting.GetMaxUserTokens()
	hasFuzzy := strings.Contains(keyword, "%") || strings.Contains(token, "%")
	if hasFuzzy {
		count, err := CountUserTokens(userId)
		if err != nil {
			common.SysLog("failed to count user tokens: " + err.Error())
			return nil, 0, errors.New("获取令牌数量失败")
		}
		if int(count) > maxTokens {
			return nil, 0, errors.New("令牌数量超过上限，仅允许精确搜索，请勿使用 % 通配符")
		}
	}

	baseQuery := tokenReadDB(DB.Model(&Token{})).Where("user_id = ?", userId)

	// 非空才加 LIKE 条件，空则跳过（不过滤该字段）
	if keyword != "" {
		keywordPattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where("name LIKE ? ESCAPE '!'", keywordPattern)
	}
	if token != "" {
		tokenPattern, err := sanitizeLikePattern(token)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where(commonKeyCol+" LIKE ? ESCAPE '!'", tokenPattern)
	}

	// 先查匹配总数（用于分页，受 maxTokens 上限保护，避免全表 COUNT）
	err = baseQuery.Limit(maxTokens).Count(&total).Error
	if err != nil {
		common.SysError("failed to count search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}

	// 再分页查数据
	err = baseQuery.Order("id desc").Offset(offset).Limit(limit).Find(&tokens).Error
	if err != nil {
		common.SysError("failed to search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}
	return tokens, total, nil
}

func ValidateUserToken(key string) (token *Token, err error) {
	if key == "" {
		return nil, ErrTokenNotProvided
	}
	token, err = GetTokenByKey(key, false)
	if err == nil {
		if token.Status == common.TokenStatusExhausted ||
			token.Status == common.TokenStatusExpired ||
			token.Status != common.TokenStatusEnabled {
			return token, ErrTokenInvalid
		}
		if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExpired
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExhausted
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		return token, nil
	}
	common.SysLog("ValidateUserToken: failed to get token: " + err.Error())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenInvalid
	}
	return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 || userId == 0 {
		return nil, errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	var err error = nil
	err = tokenReadDB(DB).First(&token, "id = ? and user_id = ?", id, userId).Error
	return &token, err
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	token := Token{Id: id}
	var err error = nil
	err = tokenReadDB(DB).First(&token, "id = ?", id).Error
	if shouldUpdateRedis(true, err) {
		gopool.Go(func() {
			if err := cacheSetToken(token); err != nil {
				common.SysLog("failed to update user status cache: " + err.Error())
			}
		})
	}
	return &token, err
}

func GetTokenByKey(key string, fromDB bool) (token *Token, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) && token != nil {
			gopool.Go(func() {
				if err := cacheSetToken(*token); err != nil {
					common.SysLog("failed to update user status cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		// Try Redis first
		token, err := cacheGetTokenByKey(key)
		if err == nil {
			return token, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = tokenReadDB(DB).Where(commonKeyCol+" = ?", key).First(&token).Error
	return token, err
}

func (token *Token) Insert() error {
	token.NormalizeRouting()
	hasCandidatesColumn := DB.Migrator().HasColumn(&Token{}, "auto_group_candidates")
	if !hasCandidatesColumn && len(token.GetAutoGroupCandidates()) > 0 {
		return ErrTokenRoutingMigrationPending
	}
	hasModelMappingColumn := hasTokenModelMappingColumn()
	if !hasModelMappingColumn && strings.TrimSpace(token.ModelMapping) != "" {
		return ErrTokenModelMappingMigrationPending
	}
	return withUserTokenCreationLock(token.UserId, func(tx *gorm.DB) error {
		return token.insertWithDB(tx, hasCandidatesColumn, hasModelMappingColumn)
	})
}

// InsertWithUserTokenLimit creates a regular token after checking the user's
// current token count under the same lock used by onboarding creation.
func (token *Token) InsertWithUserTokenLimit(maxTokens int) error {
	token.NormalizeRouting()
	hasCandidatesColumn := DB.Migrator().HasColumn(&Token{}, "auto_group_candidates")
	if !hasCandidatesColumn && len(token.GetAutoGroupCandidates()) > 0 {
		return ErrTokenRoutingMigrationPending
	}
	hasModelMappingColumn := hasTokenModelMappingColumn()
	if !hasModelMappingColumn && strings.TrimSpace(token.ModelMapping) != "" {
		return ErrTokenModelMappingMigrationPending
	}
	return withUserTokenCreationLock(token.UserId, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Token{}).Where("user_id = ?", token.UserId).Count(&count).Error; err != nil {
			return err
		}
		if int(count) >= maxTokens {
			return ErrUserTokenLimitReached
		}
		return token.insertWithDB(tx, hasCandidatesColumn, hasModelMappingColumn)
	})
}

func (token *Token) insertWithDB(tx *gorm.DB, hasCandidatesColumn bool, hasModelMappingColumn bool) error {
	omitFields := make([]string, 0, 2)
	if !hasCandidatesColumn {
		omitFields = append(omitFields, "auto_group_candidates")
	}
	if !hasModelMappingColumn {
		omitFields = append(omitFields, "model_mapping")
	}
	if len(omitFields) > 0 {
		return tx.Omit(omitFields...).Create(token).Error
	}
	return tx.Create(token).Error
}

// CreateOnboardingToken creates the first token for a user exactly once.
// The user row lock serializes onboarding requests on MySQL and PostgreSQL;
// SQLite retries its write-lock conflicts.
func CreateOnboardingToken(userId int, group string, candidates PricingGroupCandidates) (bool, error) {
	hasCandidatesColumn := DB.Migrator().HasColumn(&Token{}, "auto_group_candidates")
	if !hasCandidatesColumn && len(candidates.Values()) > 0 {
		return false, ErrTokenRoutingMigrationPending
	}
	hasModelMappingColumn := hasTokenModelMappingColumn()
	created := false
	err := withUserTokenCreationLock(userId, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Token{}).Where("user_id = ?", userId).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if int(count) >= operation_setting.GetMaxUserTokens() {
			return ErrUserTokenLimitReached
		}

		key, err := common.GenerateKey()
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		token := Token{
			UserId:              userId,
			Name:                "Мой первый ключ",
			Key:                 key,
			CreatedTime:         now,
			AccessedTime:        now,
			ExpiredTime:         -1,
			UnlimitedQuota:      true,
			Group:               group,
			AutoGroupCandidates: candidates,
		}
		if err := token.insertWithDB(tx, hasCandidatesColumn, hasModelMappingColumn); err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

func withUserTokenCreationLock(userId int, callback func(*gorm.DB) error) error {
	for attempt := 0; attempt < 5; attempt++ {
		err := DB.Transaction(func(tx *gorm.DB) error {
			var user User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userId).Error; err != nil {
				return err
			}
			return callback(tx)
		})
		if err == nil {
			return nil
		}
		if !common.UsingMainDatabase(common.DatabaseTypeSQLite) || !strings.Contains(strings.ToLower(err.Error()), "locked") {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return fmt.Errorf("creating token: database remained locked")
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (token *Token) Update() (err error) {
	token.NormalizeRouting()
	hasCandidatesColumn := DB.Migrator().HasColumn(&Token{}, "auto_group_candidates")
	if !hasCandidatesColumn && len(token.GetAutoGroupCandidates()) > 0 {
		return ErrTokenRoutingMigrationPending
	}
	hasModelMappingColumn := hasTokenModelMappingColumn()
	if !hasModelMappingColumn && strings.TrimSpace(token.ModelMapping) != "" {
		return ErrTokenModelMappingMigrationPending
	}
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheSetToken(*token)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	fields := []string{
		"name", "status", "expired_time", "remain_quota", "unlimited_quota",
		"model_limits_enabled", "model_limits", "allow_ips", "group", "cross_group_retry",
	}
	if hasModelMappingColumn {
		fields = append(fields, "model_mapping")
	}
	if hasCandidatesColumn {
		fields = append(fields, "auto_group_candidates")
	}
	err = DB.Model(token).Select(fields).Updates(token).Error
	return err
}

func (token *Token) SelectUpdate() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheSetToken(*token)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	// This can update zero values
	return DB.Model(token).Select("accessed_time", "status").Updates(token).Error
}

func (token *Token) Delete() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheDeleteToken(token.Key)
				if err != nil {
					common.SysLog("failed to delete token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Delete(token).Error
	return err
}

func (token *Token) IsModelLimitsEnabled() bool {
	return token.ModelLimitsEnabled
}

func (token *Token) GetModelLimits() []string {
	if token.ModelLimits == "" {
		return []string{}
	}
	return strings.Split(token.ModelLimits, ",")
}

func (token *Token) GetModelMapping() string {
	return token.ModelMapping
}

func (token *Token) GetModelLimitsMap() map[string]bool {
	limits := token.GetModelLimits()
	limitsMap := make(map[string]bool)
	for _, limit := range limits {
		limitsMap[limit] = true
	}
	return limitsMap
}

func (token *Token) GetAutoGroupCandidates() []string {
	return token.AutoGroupCandidates.Values()
}

func DisableModelLimits(tokenId int) error {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	token.ModelLimitsEnabled = false
	token.ModelLimits = ""
	return token.Update()
}

func DeleteTokenById(id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	err = tokenReadDB(DB).Where(token).First(&token).Error
	if err != nil {
		return err
	}
	return token.Delete()
}

func IncreaseTokenQuota(tokenId int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheIncrTokenQuota(key, int64(quota))
			if err != nil {
				common.SysLog("failed to increase token quota: " + err.Error())
			}
		})
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, tokenId, quota)
		return nil
	}
	return increaseTokenQuota(tokenId, quota)
}

func increaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

func DecreaseTokenQuota(id int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheDecrTokenQuota(key, int64(quota))
			if err != nil {
				common.SysLog("failed to decrease token quota: " + err.Error())
			}
		})
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, -quota)
		return nil
	}
	return decreaseTokenQuota(id, quota)
}

func decreaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

// CountUserTokens returns total number of tokens for the given user, used for pagination
func CountUserTokens(userId int) (int64, error) {
	var total int64
	err := DB.Model(&Token{}).Where("user_id = ?", userId).Count(&total).Error
	return total, err
}

// BatchDeleteTokens 删除指定用户的一组令牌，返回成功删除数量
func BatchDeleteTokens(ids []int, userId int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}

	tx := DB.Begin()

	var tokens []Token
	if err := tokenReadDB(tx).Where("user_id = ? AND id IN (?)", userId, ids).Find(&tokens).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Delete(&Token{}).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}

	if common.RedisEnabled {
		gopool.Go(func() {
			for _, t := range tokens {
				_ = cacheDeleteToken(t.Key)
			}
		})
	}

	return len(tokens), nil
}

func GetTokenKeysByIds(ids []int, userId int) ([]Token, error) {
	var tokens []Token
	err := DB.Select("id", commonKeyCol).
		Where("user_id = ? AND id IN (?)", userId, ids).
		Find(&tokens).Error
	return tokens, err
}

// InvalidateUserTokensCache 清理指定用户所有令牌在 Redis 中的缓存，
// 配合 InvalidateUserCache 使用，可在用户被禁用/删除时立即阻断其令牌的请求。
// 下一次请求将从数据库重新加载令牌及用户状态，从而立即识别出被禁用的用户。
func InvalidateUserTokensCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	var tokens []Token
	if err := DB.Unscoped().
		Select("id", commonKeyCol).
		Where("user_id = ?", userId).
		Find(&tokens).Error; err != nil {
		return err
	}
	var firstErr error
	for _, t := range tokens {
		if t.Key == "" {
			continue
		}
		if err := cacheDeleteToken(t.Key); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
