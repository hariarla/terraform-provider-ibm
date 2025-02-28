// Copyright IBM Corp. 2023 All Rights Reserved.
// Licensed under the Mozilla Public License v2.0

package scc_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"

	acc "github.com/IBM-Cloud/terraform-provider-ibm/ibm/acctest"
)

func TestAccIbmSccScopeCollectionDataSourceBasic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { acc.TestAccPreCheckScc(t) },
		Providers: acc.TestAccProviders,
		Steps: []resource.TestStep{
			resource.TestStep{
				Config: testAccCheckIbmSccScopeCollectionConfigBasic(acc.SccInstanceID),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.ibm_scc_scope_collection.scc_scope_collection_instance", "instance_id"),
					resource.TestCheckResourceAttrSet("data.ibm_scc_scope_collection.scc_scope_collection_instance", "scopes.#"),
					resource.TestCheckResourceAttrSet("data.ibm_scc_scope_collection.scc_scope_collection_instance", "scopes.0.id"),
					resource.TestCheckResourceAttrSet("data.ibm_scc_scope_collection.scc_scope_collection_instance", "scopes.0.name"),
				),
			},
		},
	})
}

func testAccCheckIbmSccScopeCollectionConfigBasic(instanceID string) string {
	return fmt.Sprintf(`
    data "ibm_scc_scope_collection" "scc_scope_collection_instance" {
      instance_id = "%s"
    }
	`, instanceID)
}
