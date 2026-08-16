package consensus

import (
	"testing"
)

// ==================== 过滤器创建测试 ====================

func TestNewCompositeFilter(t *testing.T) {
	// 测试创建空组合过滤器
	cf := NewCompositeFilter()
	if cf == nil {
		t.Fatal("expected non-nil CompositeFilter")
	}
	if len(cf.filters) != 0 {
		t.Errorf("expected empty filters, got %d", len(cf.filters))
	}

	// 测试创建带过滤器的组合
	cf2 := NewCompositeFilter(FilterValidEvents, FilterHighAgreement)
	if len(cf2.filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(cf2.filters))
	}
}

func TestNewOrFilter(t *testing.T) {
	// 测试创建空 OR 过滤器
	of := NewOrFilter()
	if of == nil {
		t.Fatal("expected non-nil OrFilter")
	}
	if len(of.filters) != 0 {
		t.Errorf("expected empty filters, got %d", len(of.filters))
	}

	// 测试创建带过滤器的 OR
	of2 := NewOrFilter(FilterValidEvents, FilterInvalidEvents)
	if len(of2.filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(of2.filters))
	}
}

func TestNewNotFilter(t *testing.T) {
	// 测试创建 NOT 过滤器
	nf := NewNotFilter(FilterValidEvents)
	if nf == nil {
		t.Fatal("expected non-nil NotFilter")
	}
	if nf.filter == nil {
		t.Error("expected non-nil inner filter")
	}
}

func TestEventFilterFunc(t *testing.T) {
	// 测试 EventFilterFunc 实现 EventFilter 接口
	customFilter := EventFilterFunc(func(event *BlockEvent) bool {
		return event.Priority == PriorityHigh
	})

	event := &BlockEvent{
		TaskID:   "test",
		Priority: PriorityHigh,
	}

	if !customFilter.Match(event) {
		t.Error("expected filter to match high priority event")
	}

	event.Priority = PriorityLow
	if customFilter.Match(event) {
		t.Error("expected filter not to match low priority event")
	}
}

// ==================== 过滤器匹配测试 ====================

func TestFilterValidEvents(t *testing.T) {
	tests := []struct {
		name     string
		isValid  bool
		expected bool
	}{
		{"valid event", true, true},
		{"invalid event", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:   "test",
				IsValid:  tt.isValid,
				Priority: PriorityNormal,
			}
			result := FilterValidEvents.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterInvalidEvents(t *testing.T) {
	tests := []struct {
		name     string
		isValid  bool
		expected bool
	}{
		{"valid event", true, false},
		{"invalid event", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:   "test",
				IsValid:  tt.isValid,
				Priority: PriorityNormal,
			}
			result := FilterInvalidEvents.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterHighAgreement(t *testing.T) {
	tests := []struct {
		name          string
		agreementRate float64
		expected      bool
	}{
		{"exact 0.8", 0.8, true},
		{"above 0.8", 0.9, true},
		{"below 0.8", 0.7, false},
		{"zero", 0.0, false},
		{"full agreement", 1.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:         "test",
				AgreementRate:  tt.agreementRate,
				Priority:       PriorityNormal,
			}
			result := FilterHighAgreement.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterLowAgreement(t *testing.T) {
	tests := []struct {
		name          string
		agreementRate float64
		expected      bool
	}{
		{"below 0.5", 0.4, true},
		{"exact 0.5", 0.5, false},
		{"above 0.5", 0.6, false},
		{"zero", 0.0, true},
		{"full agreement", 1.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:         "test",
				AgreementRate:  tt.agreementRate,
				Priority:       PriorityNormal,
			}
			result := FilterLowAgreement.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterUrgentPriority(t *testing.T) {
	tests := []struct {
		name     string
		priority EventPriority
		expected bool
	}{
		{"urgent priority", PriorityUrgent, true},
		{"high priority", PriorityHigh, false},
		{"normal priority", PriorityNormal, false},
		{"low priority", PriorityLow, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:   "test",
				Priority: tt.priority,
			}
			result := FilterUrgentPriority.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterHighOrUrgent(t *testing.T) {
	tests := []struct {
		name     string
		priority EventPriority
		expected bool
	}{
		{"urgent priority", PriorityUrgent, true},
		{"high priority", PriorityHigh, true},
		{"normal priority", PriorityNormal, false},
		{"low priority", PriorityLow, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:   "test",
				Priority: tt.priority,
			}
			result := FilterHighOrUrgent.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// ==================== 过滤器组合测试 ====================

func TestCompositeFilter_AllMatch(t *testing.T) {
	// 测试 AND 逻辑：所有过滤器都必须匹配
	cf := NewCompositeFilter(FilterValidEvents, FilterHighAgreement)

	tests := []struct {
		name          string
		isValid       bool
		agreementRate float64
		expected      bool
	}{
		{"both match", true, 0.9, true},
		{"only valid", true, 0.5, false},
		{"only agreement", false, 0.9, false},
		{"neither match", false, 0.5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:         "test",
				IsValid:        tt.isValid,
				AgreementRate:  tt.agreementRate,
				Priority:       PriorityNormal,
			}
			result := cf.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCompositeFilter_Empty(t *testing.T) {
	// 空组合过滤器应该匹配所有事件
	cf := NewCompositeFilter()

	event := &BlockEvent{
		TaskID:   "test",
		IsValid:  false,
		Priority: PriorityNormal,
	}

	if !cf.Match(event) {
		t.Error("expected empty composite filter to match all events")
	}
}

func TestOrFilter_AnyMatch(t *testing.T) {
	// 测试 OR 逻辑：任一过滤器匹配即可
	of := NewOrFilter(FilterValidEvents, FilterHighAgreement)

	tests := []struct {
		name          string
		isValid       bool
		agreementRate float64
		expected      bool
	}{
		{"both match", true, 0.9, true},
		{"only valid", true, 0.5, true},
		{"only agreement", false, 0.9, true},
		{"neither match", false, 0.5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:         "test",
				IsValid:        tt.isValid,
				AgreementRate:  tt.agreementRate,
				Priority:       PriorityNormal,
			}
			result := of.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestOrFilter_Empty(t *testing.T) {
	// 空 OR 过滤器应该不匹配任何事件
	of := NewOrFilter()

	event := &BlockEvent{
		TaskID:   "test",
		IsValid:  true,
		Priority: PriorityNormal,
	}

	if of.Match(event) {
		t.Error("expected empty or filter to match no events")
	}
}

func TestNotFilter_Invert(t *testing.T) {
	// 测试 NOT 逻辑：反转过滤器结果
	nf := NewNotFilter(FilterValidEvents)

	tests := []struct {
		name     string
		isValid  bool
		expected bool
	}{
		{"valid event", true, false},
		{"invalid event", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:   "test",
				IsValid:  tt.isValid,
				Priority: PriorityNormal,
			}
			result := nf.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterCombination_Complex(t *testing.T) {
	// 测试复杂组合：NOT (Valid AND HighAgreement)
	// 等价于：Invalid OR LowAgreement
	innerFilter := NewCompositeFilter(FilterValidEvents, FilterHighAgreement)
	notFilter := NewNotFilter(innerFilter)

	tests := []struct {
		name          string
		isValid       bool
		agreementRate float64
		expected      bool
	}{
		{"valid high agreement", true, 0.9, false},
		{"valid low agreement", true, 0.4, true},
		{"invalid high agreement", false, 0.9, true},
		{"invalid low agreement", false, 0.4, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:         "test",
				IsValid:        tt.isValid,
				AgreementRate:  tt.agreementRate,
				Priority:       PriorityNormal,
			}
			result := notFilter.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFilterCombination_OrWithNot(t *testing.T) {
	// 测试：Urgent OR (Valid AND HighAgreement)
	urgentOrOther := NewOrFilter(
		FilterUrgentPriority,
		NewCompositeFilter(FilterValidEvents, FilterHighAgreement),
	)

	tests := []struct {
		name          string
		isValid       bool
		agreementRate float64
		priority      EventPriority
		expected      bool
	}{
		{"urgent", false, 0.3, PriorityUrgent, true},
		{"valid and high agreement", true, 0.9, PriorityNormal, true},
		{"valid but low agreement", true, 0.4, PriorityNormal, false},
		{"none match", false, 0.4, PriorityNormal, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BlockEvent{
				TaskID:         "test",
				IsValid:        tt.isValid,
				AgreementRate:  tt.agreementRate,
				Priority:       tt.priority,
			}
			result := urgentOrOther.Match(event)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// ==================== 事件过滤测试 ====================

func TestEventBuffer_NewAndAdd(t *testing.T) {
	buf := NewEventBuffer(10)

	if buf.Len() != 0 {
		t.Errorf("expected empty buffer, got %d", buf.Len())
	}

	event := &BlockEvent{
		TaskID:         "task1",
		IsValid:        true,
		AgreementRate:  0.9,
		Priority:       PriorityNormal,
	}

	if err := buf.Add(event); err != nil {
		t.Errorf("failed to add event: %v", err)
	}

	if buf.Len() != 1 {
		t.Errorf("expected 1 event in buffer, got %d", buf.Len())
	}
}

func TestEventBuffer_GetFilteredEvents(t *testing.T) {
	buf := NewEventBuffer(10)

	// 添加多个测试事件
	events := []*BlockEvent{
		{TaskID: "task1", IsValid: true, AgreementRate: 0.9, Priority: PriorityNormal},
		{TaskID: "task2", IsValid: false, AgreementRate: 0.3, Priority: PriorityNormal},
		{TaskID: "task3", IsValid: true, AgreementRate: 0.6, Priority: PriorityHigh},
		{TaskID: "task4", IsValid: false, AgreementRate: 0.1, Priority: PriorityUrgent},
	}

	for _, e := range events {
		if err := buf.Add(e); err != nil {
			t.Fatalf("failed to add event: %v", err)
		}
	}

	// 测试过滤有效事件
	validEvents := buf.GetFilteredEvents(FilterValidEvents)
	if len(validEvents) != 2 {
		t.Errorf("expected 2 valid events, got %d", len(validEvents))
	}

	// 测试过滤高一致性事件
	highAgreementEvents := buf.GetFilteredEvents(FilterHighAgreement)
	if len(highAgreementEvents) != 1 {
		t.Errorf("expected 1 high agreement event, got %d", len(highAgreementEvents))
	}

	// 测试 nil 过滤器（返回所有事件）
	allEvents := buf.GetFilteredEvents(nil)
	if len(allEvents) != 4 {
		t.Errorf("expected 4 events with nil filter, got %d", len(allEvents))
	}
}

func TestEventBuffer_GetEventsByPriority(t *testing.T) {
	buf := NewEventBuffer(10)

	events := []*BlockEvent{
		{TaskID: "task1", Priority: PriorityHigh},
		{TaskID: "task2", Priority: PriorityNormal},
		{TaskID: "task3", Priority: PriorityHigh},
		{TaskID: "task4", Priority: PriorityUrgent},
	}

	for _, e := range events {
		if err := buf.Add(e); err != nil {
			t.Fatalf("failed to add event: %v", err)
		}
	}

	highEvents := buf.GetEventsByPriority(PriorityHigh)
	if len(highEvents) != 2 {
		t.Errorf("expected 2 high priority events, got %d", len(highEvents))
	}

	urgentEvents := buf.GetEventsByPriority(PriorityUrgent)
	if len(urgentEvents) != 1 {
		t.Errorf("expected 1 urgent event, got %d", len(urgentEvents))
	}

	lowEvents := buf.GetEventsByPriority(PriorityLow)
	if len(lowEvents) != 0 {
		t.Errorf("expected 0 low priority events, got %d", len(lowEvents))
	}
}

func TestEventBuffer_GetEventByID(t *testing.T) {
	buf := NewEventBuffer(10)

	event1 := &BlockEvent{TaskID: "task1", IsValid: true}
	event2 := &BlockEvent{TaskID: "task2", IsValid: false}

	buf.Add(event1)
	buf.Add(event2)

	found := buf.GetEventByID("task1")
	if found == nil {
		t.Error("expected to find task1")
	}
	if found.TaskID != "task1" {
		t.Errorf("expected task1, got %s", found.TaskID)
	}

	notFound := buf.GetEventByID("nonexistent")
	if notFound != nil {
		t.Error("expected nil for nonexistent task")
	}
}

func TestEventBuffer_GetEventsInRange(t *testing.T) {
	buf := NewEventBuffer(10)

	now := int64(1000)
	events := []*BlockEvent{
		{TaskID: "task1", Timestamp: now},
		{TaskID: "task2", Timestamp: now + 100},
		{TaskID: "task3", Timestamp: now + 200},
		{TaskID: "task4", Timestamp: now + 300},
	}

	for _, e := range events {
		if err := buf.Add(e); err != nil {
			t.Fatalf("failed to add event: %v", err)
		}
	}

	// 测试时间范围过滤
	rangeEvents := buf.GetEventsInRange(now+50, now+250)
	if len(rangeEvents) != 2 {
		t.Errorf("expected 2 events in range, got %d", len(rangeEvents))
	}
}

func TestEventBuffer_RemoveEvents(t *testing.T) {
	buf := NewEventBuffer(10)

	events := []*BlockEvent{
		{TaskID: "task1", IsValid: true},
		{TaskID: "task2", IsValid: false},
		{TaskID: "task3", IsValid: true},
		{TaskID: "task4", IsValid: false},
	}

	for _, e := range events {
		buf.Add(e)
	}

	if buf.Len() != 4 {
		t.Errorf("expected 4 events, got %d", buf.Len())
	}

	// 移除无效事件
	removed := buf.RemoveEvents(FilterInvalidEvents)
	if removed != 2 {
		t.Errorf("expected 2 removed events, got %d", removed)
	}

	if buf.Len() != 2 {
		t.Errorf("expected 2 events after removal, got %d", buf.Len())
	}

	// 验证剩余的是有效事件
	remaining := buf.GetEvents()
	for _, e := range remaining {
		if !e.IsValid {
			t.Error("expected only valid events to remain")
		}
	}
}

func TestEventBuffer_Clear(t *testing.T) {
	buf := NewEventBuffer(10)

	buf.Add(&BlockEvent{TaskID: "task1"})
	buf.Add(&BlockEvent{TaskID: "task2"})

	if buf.Len() != 2 {
		t.Errorf("expected 2 events, got %d", buf.Len())
	}

	buf.Clear()

	if buf.Len() != 0 {
		t.Errorf("expected 0 events after clear, got %d", buf.Len())
	}
}

func TestEventBuffer_MaxSize(t *testing.T) {
	buf := NewEventBuffer(3)

	// 添加超过最大容量的事件
	for i := 0; i < 5; i++ {
		event := &BlockEvent{TaskID: string(rune('a' + i))}
		buf.Add(event)
	}

	// 缓冲区应该保持最大大小
	if buf.Len() != 3 {
		t.Errorf("expected buffer size 3, got %d", buf.Len())
	}

	// 验证最早的事件被移除
	events := buf.GetEvents()
	if events[0].TaskID != "c" {
		t.Errorf("expected oldest event c, got %s", events[0].TaskID)
	}
}

func TestEventBuffer_Recover(t *testing.T) {
	buf1 := NewEventBuffer(10)
	buf2 := NewEventBuffer(10)

	events := []*BlockEvent{
		{TaskID: "task1", IsValid: true},
		{TaskID: "task2", IsValid: false},
	}

	for _, e := range events {
		buf1.Add(e)
	}

	// 从 buf1 恢复到 buf2
	recovered := buf2.Recover(buf1)
	if recovered != 2 {
		t.Errorf("expected 2 recovered events, got %d", recovered)
	}

	if buf2.Len() != 2 {
		t.Errorf("expected 2 events in recovered buffer, got %d", buf2.Len())
	}
}

func TestEventBuffer_RecoverFromNil(t *testing.T) {
	buf := NewEventBuffer(10)

	// 从 nil 恢复应该返回 0
	recovered := buf.Recover(nil)
	if recovered != 0 {
		t.Errorf("expected 0 recovered from nil, got %d", recovered)
	}
}

func TestPriorityEventQueue_Filter(t *testing.T) {
	// 测试优先级队列与过滤器的结合
	queue := NewPriorityEventQueue()

	events := []*BlockEvent{
		{TaskID: "task1", Priority: PriorityLow},
		{TaskID: "task2", Priority: PriorityHigh},
		{TaskID: "task3", Priority: PriorityUrgent},
		{TaskID: "task4", Priority: PriorityNormal},
	}

	for _, e := range events {
		queue.Push(e)
	}

	// 使用高优先级过滤器过滤
	highPriorityFilter := NewCompositeFilter(FilterHighOrUrgent)
	filtered := make([]*BlockEvent, 0)

	for queue.Len() > 0 {
		event := queue.TryPop()
		if event != nil && highPriorityFilter.Match(event) {
			filtered = append(filtered, event)
		}
	}

	if len(filtered) != 2 {
		t.Errorf("expected 2 high/urgent priority events, got %d", len(filtered))
	}
}

func TestEventFilterInterface(t *testing.T) {
	// 测试 EventFilter 接口的各种实现
	var filter EventFilter

	// 测试 EventFilterFunc 实现接口
	filter = EventFilterFunc(func(e *BlockEvent) bool {
		return e.IsValid && e.AgreementRate >= 0.8
	})

	event := &BlockEvent{IsValid: true, AgreementRate: 0.9}
	if !filter.Match(event) {
		t.Error("expected EventFilterFunc to match")
	}

	// 测试 CompositeFilter 实现接口
	filter = NewCompositeFilter(FilterValidEvents, FilterHighAgreement)
	if !filter.Match(event) {
		t.Error("expected CompositeFilter to match")
	}

	// 测试 OrFilter 实现接口
	filter = NewOrFilter(FilterInvalidEvents, FilterHighAgreement)
	if !filter.Match(event) {
		t.Error("expected OrFilter to match")
	}

	// 测试 NotFilter 实现接口
	filter = NewNotFilter(FilterInvalidEvents)
	if !filter.Match(event) {
		t.Error("expected NotFilter to match")
	}
}
