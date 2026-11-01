/*
 * @Author: kamalyes example@qq.com
 * @Date: 2026-11-01 17:05:08
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 19:28:30
 * @FilePath: \kronos-my-service\models\base.go
 * @Description: 模型基础类型定义，提供通用数据类型和工具方法
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package models

import (
	"database/sql/driver"

	"github.com/kamalyes/go-pbmo"
	sqlbuilder "github.com/kamalyes/go-sqlbuilder"
	commonpb "github.com/kamalyes/kronos-share-proto-template/pb/common"
)

// BuildPagination 构建分页响应
func BuildPagination(page, size, total int32) *commonpb.Paging {
	return &commonpb.Paging{
		Page:  page,
		Size:  size,
		Total: total,
	}
}

// PBConvertible 定义了将模型转换为proto消息的方法
type PBConvertible interface {
	NewConverter() *pbmo.BidiConverter
}

// LocalizedTextList 国际化文本列表类型
type LocalizedTextList []commonpb.LocalizedText

// Get 根据语言键获取对应的翻译文本
func (l LocalizedTextList) Get(key string) string {
	for i := range l {
		t := &l[i]
		if t.Key == key {
			return t.Value
		}
	}
	return ""
}

// Value 实现 driver.Valuer 接口
func (l LocalizedTextList) Value() (driver.Value, error) {
	return sqlbuilder.Slice[commonpb.LocalizedText](l).Value()
}

// Scan 实现 sql.Scanner 接口
func (l *LocalizedTextList) Scan(value interface{}) error {
	var slice sqlbuilder.Slice[commonpb.LocalizedText]
	if err := slice.Scan(value); err != nil {
		return err
	}
	*l = LocalizedTextList(slice)
	return nil
}

// ConvertI18NNamesToModel 将proto中的国际化名称列表转换为模型中的LocalizedTextList
func ConvertI18NNamesToModel(names []*commonpb.LocalizedText) LocalizedTextList {
	if names == nil {
		return nil
	}
	result := make(LocalizedTextList, 0, len(names))
	for _, name := range names {
		result = append(result, commonpb.LocalizedText{
			Key:   name.Key,
			Value: name.Value,
		})
	}
	return result
}

// ConvertI18NNamesToPB 将模型中的LocalizedTextList转换为proto中的国际化名称列表
func ConvertI18NNamesToPB(names LocalizedTextList) []*commonpb.LocalizedText {
	if names == nil {
		return nil
	}
	result := make([]*commonpb.LocalizedText, 0, len(names))
	for i := range names {
		result = append(result, &commonpb.LocalizedText{
			Key:   names[i].Key,
			Value: names[i].Value,
		})
	}
	return result
}
