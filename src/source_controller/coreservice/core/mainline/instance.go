/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.,
 * Copyright (C) 2017-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the ",License",); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an ",AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mainline

import (
	"encoding/json"
	"fmt"

	"configcenter/src/common/blog"
	"configcenter/src/common/metadata"
)

// SearchMainlineBusinessTopo get topo tree of mainline model
func (m *topoManager) SearchMainlineInstanceTopo(bkBizID int64, withDetail bool) (*metadata.TopoInstanceNode, error) {

	bizTopoNode, err := m.SearchMainlineModelTopo(false)
	if err != nil {
		blog.Errorf("get mainline model topo info failed, %+v", err)
		return nil, fmt.Errorf("get mainline model topo info failed, %+v", err)
	}
	mainline, err := json.Marshal(bizTopoNode)
	if err != nil {
		blog.Errorf("get other mainline instances by business:%d failed, %+v", bkBizID, err)
		return nil, fmt.Errorf("dump model mainline data failed, err: %+v", err)
	}
	blog.V(9).Infof("model mainline: %s", mainline)

	im, err := NewInstanceMainline(m.DbProxy, bkBizID)
	if err != nil {
		blog.Errorf("SearchMainlineInstanceTopo failed, NewInstanceMainline failed, bizID: %d, err: %+v", bkBizID, err)
		return nil, fmt.Errorf("new mainline instance by business:%d failed, %+v", bkBizID, err)
	}

	im.SetModelTree(bizTopoNode)
	im.LoadModelParentMap()

	if err := im.LoadSetInstances(); err != nil {
		blog.Errorf("get set instances by business:%d failed, %+v", bkBizID, err)
		return nil, fmt.Errorf("get set instances by business:%d failed, %+v", bkBizID, err)
	}

	if err := im.LoadModuleInstances(); err != nil {
		blog.Errorf("get module instances by business:%d failed, %+v", bkBizID, err)
		return nil, fmt.Errorf("get module instances by business:%d failed, %+v", bkBizID, err)
	}

	if err := im.LoadMainlineInstances(); err != nil {
		blog.Errorf("get other mainline instances by business:%d failed, %+v", bkBizID, err)
		return nil, fmt.Errorf("get other mainline instances by business:%d failed, %+v", bkBizID, err)
	}

	if err := im.ConstructBizTopoInstance(withDetail); err != nil {
		blog.Errorf("construct business:%d detail as topo instance failed, %+v", bkBizID, err)
		return nil, fmt.Errorf("construct business:%d detail as topo instance failed, %+v", bkBizID, err)
	}

	if err := im.OrganizeSetInstance(withDetail); err != nil {
		blog.Errorf("organize set instance failed, businessID:%d, %+v", bkBizID, err)
		return nil, fmt.Errorf("organize set instance failed, businessID:%d, %+v", bkBizID, err)
	}

	if err := im.OrganizeModuleInstance(withDetail); err != nil {
		blog.Errorf("organize module instance failed, businessID:%d, %+v", bkBizID, err)
		return nil, fmt.Errorf("organize module instance failed, businessID:%d, %+v", bkBizID, err)
	}

	if err := im.OrganizeMainlineInstance(withDetail); err != nil {
		blog.Errorf("organize other mainline instance failed, businessID:%d, %+v", bkBizID, err)
		return nil, fmt.Errorf("organize other mainline instance failed, businessID:%d, %+v", bkBizID, err)
	}

	instanceMap := im.GetInstanceMap()
	instanceMapStr, err := json.Marshal(instanceMap)
	if err != nil {
		blog.Errorf("json encode instanceMap:%+v failed, %+v", instanceMap, err)
		return nil, fmt.Errorf("json encode instanceMap:%+v failed, %+v", instanceMap, err)
	}
	blog.V(3).Infof("instanceMap before check is: %s", instanceMapStr)

	if err := im.CheckAndFillingMissingModels(withDetail); err != nil {
		blog.Errorf("check and filling missing models failed, business:%d %+v", bkBizID, err)
		return nil, fmt.Errorf("check and filling missing models failed, business:%d %+v", bkBizID, err)
	}

	instanceMapStr, err = json.Marshal(im.GetInstanceMap())
	if err != nil {
		blog.Errorf("json encode instanceMap failed, %+v", err)
		return nil, fmt.Errorf("json encode instanceMap failed, %+v", err)
	}
	blog.V(3).Infof("instanceMap after check: %s", instanceMapStr)

	if err := im.ConstructInstanceTopoTree(withDetail); err != nil {
		blog.Errorf("get other mainline instances by business:%d failed, %+v", bkBizID, err)
		return nil, fmt.Errorf("get other mainline instances by business:%d failed, %+v", bkBizID, err)
	}

	root := im.GetRoot()
	blog.V(9).Infof("topo instance tree root is: %+v", root)
	treeData, err := json.Marshal(root)
	if err != nil {
		blog.Errorf("get other mainline instances by business:%d failed, %+v", bkBizID, err)
		return root, nil
	}
	blog.V(9).Infof("topo instance tree root data is: %s", treeData)
	return root, nil
}
