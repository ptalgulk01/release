import { Operator, project } from "../../views/netobserv"
import { netflowPage, querySumSelectors, topologySelectors } from "../../views/netflow-page"
import { dashboard, graphSelector } from "views/dashboards-page"

const metricType = [
    "Bytes",
    "Packets",
    "Dropped bytes",
    "Dropped packets"
]

const PacketDropPanels = [
    // below panel should appear with the 'node_drop_packets_total' metric
    "top-drops-per-node-(pps)-chart",
    // below panel should appear with the 'node_drop_bytes_total' metric
    "top-drops-per-node-(bps)-chart",
    // below panel should appear with the 'namespace_drop_packets_total' metric
    "top-drops-per-infra-namespace-(pps)-chart",
    // below panel should appear with the 'namespace_drop_bytes_total' metric
    "top-drops-per-infra-namespace-(bps)-chart",
    // below panel should appear with the 'workload_drop_packets_total' metric
    "top-drops-per-infra-workload-(pps)-chart",
    // below panel should appear with the 'workload_drop_bytes_total' metric
    "top-drops-per-infra-workload-(bps)-chart"
]

describe('(OCP-66141 Network_Observability) PacketDrop dashboards test', { tags: ['Network_Observability'] }, function () {
    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        Operator.install()
        Operator.createFlowcollector(project, "PacketDrop")
    })

    it("(OCP-66141, aramesha, Network_Observability) Validate PacketDrop edge labels and Query Summary stats", function () {
        netflowPage.visit()
        cy.get('#tabs-container li:nth-child(3)').click()
        cy.get('#drawer').should('not.be.empty')

        cy.byTestID("show-view-options-button").should('exist').click().then(views => {
            cy.contains('Display options').should('exist').click()
            // set one display to test with
            cy.byTestID('layout-dropdown').click()
            cy.byTestID('Grid').click()
        })

        // update metricType to Dropped bytes
        cy.byTestID(topologySelectors.metricTypeDrop).should('exist').click()
        cy.get(topologySelectors.metricType).find('li').should('have.length', 4).each((item, index) => {
            cy.wrap(item).should('contain.text', metricType[index])
        })
        cy.get('#PktDropBytes').click()

        // verify Query Summary stats for Dropped Bytes metric
        cy.get(querySumSelectors.droppedBytesCount).should('exist').then(droppedBytesCnt => {
            cy.checkQuerySummary(droppedBytesCnt)
        })

        cy.get(querySumSelectors.droppedBpsCount).should('exist').then(droppedBpsCnt => {
            cy.checkQuerySummary(droppedBpsCnt)
        })

        cy.get(querySumSelectors.droppedPacketsCount).should('exist').then(droppedPacketsCnt => {
            cy.checkQuerySummary(droppedPacketsCnt)
        })

        // update metricType to Dropped packets
        cy.byTestID(topologySelectors.metricTypeDrop).should('exist').click()
        cy.get('#PktDropPackets').click()

        // verify Query Summary stats for Dropped Bytes metric
        cy.get(querySumSelectors.droppedBytesCount).should('exist').then(droppedBytesCnt => {
            cy.checkQuerySummary(droppedBytesCnt)
        })

        cy.get(querySumSelectors.droppedBpsCount).should('exist').then(droppedBpsCnt => {
            cy.checkQuerySummary(droppedBpsCnt)
        })

        cy.get(querySumSelectors.droppedPacketsCount).should('exist').then(droppedPacketsCnt => {
            cy.checkQuerySummary(droppedPacketsCnt)
        })
        netflowPage.resetClearFilters()
    })

    it("(OCP-66141, aramesha, Network_Observability) Validate packetDrop dashboards", function () {
        // navigate to 'NetObserv / Main' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("netobserv-main")

        // verify 'Drops' panel
        cy.get('[data-test="drops-chart"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        cy.checkDashboards(PacketDropPanels)
    })

    after("Delete flowcollector", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
