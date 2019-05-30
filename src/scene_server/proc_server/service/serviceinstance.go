/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.
 * Copyright (C) 2017-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package service

import (
	"configcenter/src/common"
	"configcenter/src/common/blog"
	"configcenter/src/common/http/rest"
	"configcenter/src/common/metadata"
)

// create service instance batch, which must belongs to a same module and service template.
// if needed, it also create process instance for a service instance at the same time.
func (ps *ProcServer) CreateServiceInstances(ctx *rest.Contexts) {
	input := new(metadata.CreateServiceInstanceForServiceTemplateInput)
	if err := ctx.DecodeInto(input); err != nil {
		ctx.RespAutoError(err)
		return
	}

	_, err := metadata.BizIDFromMetadata(input.Metadata)
	if err != nil {
		ctx.RespErrorCodeOnly(common.CCErrCommHTTPInputInvalid,
			"create service instance for template: %d, moduleID: %d, but get business id failed, err: %v",
			input.TemplateID, input.ModuleID, err)
		return
	}

	serviceInstanceIDs := make([]int64, 0)
	for _, inst := range input.Instances {
		instance := &metadata.ServiceInstance{
			Metadata:          input.Metadata,
			Name:              input.Name,
			ServiceTemplateID: input.TemplateID,
			ModuleID:          input.ModuleID,
			HostID:            inst.HostID,
		}

		// create service instance at first
		temp, err := ps.CoreAPI.CoreService().Process().CreateServiceInstance(ctx.Kit.Ctx, ctx.Kit.Header, instance)
		if err != nil {
			ctx.RespWithError(err, common.CCErrCommHTTPDoRequestFailed,
				"create service instance for template: %d, moduleID: %d, failed, err: %v",
				input.TemplateID, input.ModuleID, err)
			return
		}

		// if this service have process instance to create, then create it now.
		for _, detail := range inst.Processes {
			id, err := ps.Logic.CreateProcessInstance(ctx.Kit, &detail.ProcessInfo)
			if err != nil {
				ctx.RespWithError(err, common.CCErrProcCreateProcessFailed,
					"create service instance, for template: %d, moduleID: %d, but create process failed, err: %v",
					input.TemplateID, input.ModuleID, err)
				return
			}

			relation := &metadata.ProcessInstanceRelation{
				Metadata:          input.Metadata,
				ProcessID:         int64(id),
				ProcessTemplateID: detail.ProcessTemplateID,
				ServiceInstanceID: temp.ID,
				HostID:            inst.HostID,
			}

			_, err = ps.CoreAPI.CoreService().Process().CreateProcessInstanceRelation(ctx.Kit.Ctx, ctx.Kit.Header, relation)
			if err != nil {
				ctx.RespWithError(err, common.CCErrProcCreateProcessFailed,
					"create service instance relations, for template: %d, moduleID: %d, err: %v",
					input.TemplateID, input.ModuleID, err)
				return
			}
		}

		serviceInstanceIDs = append(serviceInstanceIDs, temp.ID)
	}

	ctx.RespEntity(metadata.NewSuccessResp(serviceInstanceIDs))
}

func (ps *ProcServer) DeleteProcessInstanceInServiceInstance(ctx *rest.Contexts) {
	input := new(metadata.DeleteProcessInstanceInServiceInstanceInput)
	if err := ctx.DecodeInto(input); err != nil {
		ctx.RespAutoError(err)
		return
	}

	_, err := metadata.BizIDFromMetadata(input.Metadata)
	if err != nil {
		ctx.RespErrorCodeOnly(common.CCErrCommHTTPInputInvalid, "delete process instance in service instance failed, err: %v", err)
		return
	}

	if err := ps.Logic.DeleteProcessInstanceBatch(ctx.Kit, input.ProcessInstanceIDs); err != nil {
		ctx.RespWithError(err, common.CCErrProcDeleteProcessFailed, "delete process instance:%v failed, err: %v", input.ProcessInstanceIDs, err)
		return
	}

	ctx.RespEntity(metadata.NewSuccessResp(nil))
}

func (ps *ProcServer) GetServiceInstancesInModule(ctx *rest.Contexts) {
	input := new(metadata.GetServiceInstanceInModuleInput)
	if err := ctx.DecodeInto(input); err != nil {
		ctx.RespAutoError(err)
		return
	}

	bizID, err := metadata.BizIDFromMetadata(input.Metadata)
	if err != nil {
		ctx.RespErrorCodeOnly(common.CCErrCommHTTPInputInvalid, "get service instances in module, but parse biz id failed, err: %v", err)
		return
	}

	option := &metadata.ListServiceInstanceOption{
		BusinessID: bizID,
		ModuleID:   input.ModuleID,
		Page:       input.Page,
	}
	instances, err := ps.CoreAPI.CoreService().Process().ListServiceInstance(ctx.Kit.Ctx, ctx.Kit.Header, option)
	if err != nil {
		ctx.RespWithError(err, common.CCErrProcGetServiceInstancesFailed, "get service instance in module: %d failed, err: %v", input.ModuleID, err)
		return
	}

	ctx.RespEntity(metadata.NewSuccessResp(instances))
}

func (ps *ProcServer) DeleteServiceInstance(ctx *rest.Contexts) {
	input := new(metadata.DeleteServiceInstanceOption)
	if err := ctx.DecodeInto(input); err != nil {
		ctx.RespAutoError(err)
		return
	}

	_, err := metadata.BizIDFromMetadata(input.Metadata)
	if err != nil {
		ctx.RespErrorCodeOnly(common.CCErrCommHTTPInputInvalid, "delete service instances, but parse biz id failed, err: %v", err)
		return
	}

	err = ps.CoreAPI.CoreService().Process().DeleteServiceInstance(ctx.Kit.Ctx, ctx.Kit.Header, input.ServiceInstanceID)
	if err != nil {
		ctx.RespWithError(err, common.CCErrProcDeleteServiceInstancesFailed, "delete service instance: %d failed, err: %v", input.ServiceInstanceID, err)
		return
	}

	ctx.RespEntity(metadata.NewSuccessResp(nil))
}

// this function works to find differences between the service template and service instances in a module.
// compared to the service template's process template, a process instance in the service instance may
// contains several differences, like as follows:
// unchanged: the process instance's property values are same with the process template it belongs.
// changed: the process instance's property values are not same with the process template it belongs.
// add: a new process template is added, compared to the service instance belongs to this service template.
// deleted: a process is already deleted, compared to the service instance belongs to this service template.
func (ps *ProcServer) FindDifferencesBetweenServiceAndProcessInstance(ctx *rest.Contexts) {
	input := new(metadata.FindServiceTemplateAndInstanceDifferenceOption)
	if err := ctx.DecodeInto(input); err != nil {
		ctx.RespAutoError(err)
		return
	}

	bizID, err := metadata.BizIDFromMetadata(input.Metadata)
	if err != nil {
		ctx.RespErrorCodeOnly(common.CCErrCommHTTPInputInvalid, "find difference between service template and process instances, but parse biz id failed, err: %v", err)
		return
	}

	// step 1:
	// find process object's attribute
	attrResult, err := ps.CoreAPI.CoreService().Model().ReadModelAttr(ctx.Kit.Ctx, ctx.Kit.Header, common.BKInnerObjIDProc, new(metadata.QueryCondition))
	if err != nil {
		ctx.RespWithError(err, common.CCErrProcGetProcessTemplatesFailed,
			"find difference between service template: %d and process instances, bizID: %d, but get process attributes failed, err: %v",
			input.ServiceTemplateID, bizID, err)
		return
	}

	attributeMap := make(map[string]metadata.Attribute)
	for _, attr := range attrResult.Data.Info {
		attributeMap[attr.PropertyID] = attr
	}

	// step 2:
	// find all the process template in this service template, for compare usage.
	listProcOption := &metadata.ListProcessTemplatesOption{
		BusinessID:        bizID,
		ServiceTemplateID: input.ServiceTemplateID,
	}
	processTemplates, err := ps.CoreAPI.CoreService().Process().ListProcessTemplates(ctx.Kit.Ctx, ctx.Kit.Header, listProcOption)
	if err != nil {
		ctx.RespWithError(err, common.CCErrProcGetProcessTemplatesFailed,
			"find difference between service template: %d and process instances, bizID: %d, but get process templates failed, err: %v",
			input.ServiceTemplateID, bizID, err)
		return
	}

	// step 3:
	// find process instance's relations, which allows us know the relationship between
	// process instance and it's template, service instance, etc.
	pTemplateMap := make(map[int64]*metadata.ProcessTemplate)
	serviceRelationMap := make(map[int64][]metadata.ProcessInstanceRelation)
	for _, pTemplate := range processTemplates.Info {
		pTemplateMap[pTemplate.ID] = &pTemplate

		option := metadata.ListProcessInstanceRelationOption{
			BusinessID:        bizID,
			ProcessTemplateID: pTemplate.ID,
		}

		relations, err := ps.CoreAPI.CoreService().Process().ListProcessInstanceRelation(ctx.Kit.Ctx, ctx.Kit.Header, &option)
		if err != nil {
			ctx.RespWithError(err, common.CCErrProcGetProcessInstanceRelationFailed,
				"find difference between service template: %d and process instances, bizID: %d, moduleID: %d, but get service instance relations failed, err: %v",
				input.ServiceTemplateID, bizID, input.ModuleID, err)
			return
		}

		for _, r := range relations.Info {
			serviceRelationMap[r.ServiceInstanceID] = append(serviceRelationMap[r.ServiceInstanceID], r)
		}

	}

	// step 4:
	// find all the service instances belongs to this service template and this module.
	// which contains the process instances details at the same time.
	serviceOption := &metadata.ListServiceInstanceOption{
		BusinessID:        bizID,
		ServiceTemplateID: input.ServiceTemplateID,
		ModuleID:          input.ModuleID,
	}
	serviceInstances, err := ps.CoreAPI.CoreService().Process().ListServiceInstance(ctx.Kit.Ctx, ctx.Kit.Header, serviceOption)
	if err != nil {
		ctx.RespWithError(err, common.CCErrProcGetServiceInstancesFailed,
			"find difference between service template: %d and process instances, bizID: %d, moduleID: %d, but get service instance failed, err: %v",
			input.ServiceTemplateID, bizID, input.ModuleID, err)
		return
	}

	// step 5: compare the process instance with it's process template one by one in a service instance.
	differences := make([]*metadata.ServiceProcessInstanceDifference, 0)
	processTemplatesUsing := make(map[int64]bool)
	for _, serviceInstance := range serviceInstances.Info {
		// get the process instance relation
		relations := serviceRelationMap[serviceInstance.ID]

		if len(relations) == 0 {
			// There is no relations in this service instance, which means no process instances.
			// Normally, this can not be happy.
			// TODO: what???
			differences = append(differences, &metadata.ServiceProcessInstanceDifference{
				ServiceInstanceID:   serviceInstance.ID,
				ServiceInstanceName: serviceInstance.Name,
				HostID:              serviceInstance.HostID,
				Differences:         metadata.NewDifferenceDetail(),
			})
			continue
		}

		// now, we can compare the differences between process template and process instance.
		diff := &metadata.ServiceProcessInstanceDifference{
			ServiceInstanceID:   serviceInstance.ID,
			ServiceInstanceName: serviceInstance.Name,
			HostID:              serviceInstance.HostID,
			Differences:         metadata.NewDifferenceDetail(),
		}
		for _, r := range relations {
			// remember what process template is using, so that we can check whether a new process template has
			// been added or not.
			processTemplatesUsing[r.ProcessTemplateID] = true

			// find the process instance now.
			processInstance, err := ps.Logic.GetProcessInstanceWithID(ctx.Kit, r.ProcessID)
			if err != nil {
				ctx.RespWithError(err, common.CCErrProcGetProcessInstanceFailed,
					"find difference between service template: %d and process instances, bizID: %d, moduleID: %d, but get process instance: %d failed, err: %v",
					input.ServiceTemplateID, bizID, input.ModuleID, r.ProcessID, err)
				return
			}

			// let's check if the process instance bounded process template is still exist in it's service template
			// if not exist, that means that this process has already been removed from service template.
			pTemplate, exist := pTemplateMap[r.ProcessTemplateID]
			if !exist {
				// the process instance's bounded process template has already been removed from this service template.
				diff.Differences.Removed = append(diff.Differences.Removed, metadata.ProcessDifferenceDetail{
					ProcessTemplateID: r.ProcessTemplateID,
					ProcessInstance:   *processInstance,
				})
				differences = append(differences, diff)
				continue
			}

			diff := &metadata.ServiceProcessInstanceDifference{
				ServiceInstanceID:   serviceInstance.ID,
				ServiceInstanceName: serviceInstance.Name,
				HostID:              serviceInstance.HostID,
				Differences:         metadata.NewDifferenceDetail(),
			}

			if pTemplate.Property == nil {
				continue
			}

			diffAttributes := ps.Logic.GetDifferenceInProcessTemplateAndInstance(pTemplate.Property, processInstance, attributeMap)
			if len(diffAttributes) == 0 {
				// the process instance's value is exactly same with the process template's value
				diff.Differences.Unchanged = append(diff.Differences.Unchanged, metadata.ProcessDifferenceDetail{
					ProcessTemplateID: pTemplate.ID,
					ProcessInstance:   *processInstance,
				})
			} else {
				// the process instance's value is not same with the process template's value
				diff.Differences.Changed = append(diff.Differences.Changed, metadata.ProcessDifferenceDetail{
					ProcessTemplateID: pTemplate.ID,
					ProcessInstance:   *processInstance,
					ChangedAttributes: diffAttributes,
				})
			}

		}

		// it's time to see whether a new process template has been added.
		for _, t := range processTemplates.Info {
			if _, exist := processTemplatesUsing[t.ID]; exist {
				continue
			}

			// this process template does not exist in this template's all service instances.
			// so it's a new one to be added.
			if t.Property == nil {
				continue
			}
			diff.Differences.Added = append(diff.Differences.Added, metadata.ProcessDifferenceDetail{
				ProcessTemplateID: t.ID,
				ProcessInstance:   *ps.Logic.NewProcessInstanceFromProcessTemplate(t.Property),
			})

		}

		differences = append(differences, diff)
	}

	ctx.RespEntity(differences)
}

// Force sync the service instance with it's bounded service template.
// It keeps the processes exactly same with the process template in the service template,
// which means the number of process is same, and the process instance's info is also exactly same.
// It contains several scenarios in a service instance:
// 1. add a new process
// 2. update a process
// 3. removed a process

func (ps *ProcServer) ForceSyncServiceInstanceAccordingToServiceTemplate(ctx *rest.Contexts) {
	input := new(metadata.ForceSyncServiceInstanceWithTemplateInput)
	if err := ctx.DecodeInto(input); err != nil {
		ctx.RespAutoError(err)
		return
	}

	bizID, err := metadata.BizIDFromMetadata(input.Metadata)
	if err != nil {
		ctx.RespErrorCodeOnly(common.CCErrCommHTTPInputInvalid,
			"force sync service instance according to service template, but parse biz id failed, err: %v", err)
		return
	}

	// step 1:
	// find all the process template according to the service template id
	option := &metadata.ListProcessTemplatesOption{
		BusinessID:        bizID,
		ServiceTemplateID: input.ServiceTemplateID,
	}
	processTemplate, err := ps.CoreAPI.CoreService().Process().ListProcessTemplates(ctx.Kit.Ctx, ctx.Kit.Header, option)
	if err != nil {
		ctx.RespWithError(err, common.CCErrProcGetProcessTemplatesFailed,
			"force sync service instance according to service template: %d, but list process template failed, err: %v",
			input.ServiceTemplateID, err)
		return

	}
	processTemplateMap := make(map[int64]*metadata.ProcessTemplate)
	for _, t := range processTemplate.Info {
		processTemplateMap[t.ID] = &t
	}

	// step2:
	// find all the process instances relations for the usage of getting process instances.
	relationOption := &metadata.ListProcessInstanceRelationOption{
		BusinessID:        bizID,
		ServiceInstanceID: input.ServiceInstances,
	}
	relations, err := ps.CoreAPI.CoreService().Process().ListProcessInstanceRelation(ctx.Kit.Ctx, ctx.Kit.Header, relationOption)
	if err != nil {
		ctx.RespWithError(err, common.CCErrProcGetProcessInstanceRelationFailed,
			"force sync service instance according to service template: %d, but list process template failed, err: %v",
			input.ServiceTemplateID, err)
		return
	}
	procIDs := make([]int64, 0)
	for _, r := range relations.Info {
		procIDs = append(procIDs, r.ProcessID)
	}

	// step 3:
	// find all the process instance in process instance relation.
	processInstances, err := ps.Logic.ListProcessInstanceWithIDs(ctx.Kit, procIDs)
	if err != nil {
		ctx.RespWithError(err, common.CCErrProcGetProcessInstanceFailed,
			"force sync service instance according to service template: %d, but list process instance: %v failed, err: %v",
			input.ServiceTemplateID, procIDs, err)
		return
	}
	processInstanceMap := make(map[int64]*metadata.Process)
	for _, p := range processInstances {
		processInstanceMap[p.ProcessID] = &p
	}

	// step 4:
	// rearrange the service instance with process instance.
	serviceInstanceWithProcessMap := make(map[int64][]*metadata.Process)
	serviceInstanceWithTemplateMap := make(map[int64]map[int64]bool)
	serviceInstanceWithHostMap := make(map[int64]int64)
	processInstanceWithTemplateMap := make(map[int64]int64)
	for _, r := range relations.Info {
		p, exist := processInstanceMap[r.ProcessID]
		if !exist {
			// something is wrong, but can this process instance,
			// but we can find it in the process instance relation.
			blog.Warnf("force sync service instance according to service template: %d, but can not find the process instance: %d",
				input.ServiceTemplateID, r.ProcessID)
			continue
		}
		if _, exist := serviceInstanceWithProcessMap[r.ServiceInstanceID]; !exist {
			serviceInstanceWithProcessMap[r.ServiceInstanceID] = make([]*metadata.Process, 0)
		}
		serviceInstanceWithProcessMap[r.ServiceInstanceID] = append(serviceInstanceWithProcessMap[r.ServiceInstanceID], p)
		processInstanceWithTemplateMap[r.ProcessID] = r.ProcessTemplateID
		serviceInstanceWithHostMap[r.ServiceInstanceID] = r.HostID

		if _, exist := serviceInstanceWithTemplateMap[r.ServiceInstanceID][r.ProcessTemplateID]; !exist {
			serviceInstanceWithTemplateMap[r.ServiceInstanceID] = make(map[int64]bool)
		}
		serviceInstanceWithTemplateMap[r.ServiceInstanceID][r.ProcessTemplateID] = true
	}

	// step 5:
	// compare the difference between process instance and process template from one service instance to another.
	for svcInstanceID, processes := range serviceInstanceWithProcessMap {
		for _, process := range processes {
			template, exist := processTemplateMap[processInstanceWithTemplateMap[process.ProcessID]]
			if !exist {
				// this process template has already removed form the service template,
				// which means this process instance need to be removed from this service instance
				if err := ps.Logic.DeleteProcessInstance(ctx.Kit, process.ProcessID); err != nil {
					ctx.RespWithError(err, common.CCErrProcDeleteProcessFailed,
						"force sync service instance according to service template: %d, but delete process instance: %d with template: %d failed, err: %v",
						input.ServiceTemplateID, process.ProcessID, template.ID, err)
					return
				}

				// remove process instance relation now.
				if err := ps.CoreAPI.CoreService().Process().DeleteProcessInstanceRelation(ctx.Kit.Ctx, ctx.Kit.Header, process.ProcessID); err != nil {
					ctx.RespWithError(err, common.CCErrProcDeleteProcessFailed,
						"force sync service instance according to service template: %d, but delete process instance relation: %d with template: %d failed, err: %v",
						input.ServiceTemplateID, process.ProcessID, template.ID, err)
				}
				continue
			}

			// this process's bounded is still exist, need to check whether this process instance
			// need to be updated or not.
			proc, changed := ps.Logic.CheckProcessTemplateAndInstanceIsDifferent(template.Property, process)
			if !changed {
				// nothing is changed.
				continue
			}

			// process template has already changed, this process instance need to be updated.
			if err := ps.Logic.UpdateProcessInstance(ctx.Kit, process.ProcessID, proc); err != nil {
				ctx.RespWithError(err, common.CCErrProcUpdateProcessFailed,
					"force sync service instance according to service template: %d, service instance: %d, but update process instance with template: %d failed, err: %v, process: %v",
					input.ServiceTemplateID, svcInstanceID, template.ID, err, proc)
				return
			}
		}
	}

	// step 6:
	// check if a new process is added to the service template.
	// if true, then create a new process instance for every service instance with process template's default value.
	for id, pt := range processTemplateMap {
		for svcID, templates := range serviceInstanceWithTemplateMap {
			if _, exist := templates[id]; exist {
				// nothing is changed.
				continue
			}

			// we can not find this process template in all this service instance,
			// which means that a new process template need to be added to this service instance
			process, err := ps.Logic.CreateProcessInstance(ctx.Kit, ps.Logic.NewProcessInstanceFromProcessTemplate(pt.Property))
			if err != nil {
				ctx.RespWithError(err, common.CCErrProcCreateProcessFailed,
					"force sync service instance according to service template: %d, but create process instance with template: %d failed, err: %v",
					input.ServiceTemplateID, id, err)
				return
			}

			relation := &metadata.ProcessInstanceRelation{
				Metadata:          input.Metadata,
				ProcessID:         int64(process),
				ServiceInstanceID: svcID,
				ProcessTemplateID: id,
				HostID:            serviceInstanceWithHostMap[svcID],
			}

			// create service instance relation, so that the process instance created upper can be related to this service instance.
			_, err = ps.CoreAPI.CoreService().Process().CreateProcessInstanceRelation(ctx.Kit.Ctx, ctx.Kit.Header, relation)
			if err != nil {
				ctx.RespWithError(err, common.CCErrProcCreateProcessFailed,
					"force sync service instance according to service template: %d, but create process instance relation with template: %d failed, err: %v",
					input.ServiceTemplateID, id, err)
				return
			}

		}
	}

	// Finally, we do the force sync successfully.
	ctx.RespEntity(metadata.NewSuccessResp(nil))
}