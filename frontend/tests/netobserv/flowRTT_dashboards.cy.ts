import { Operator, project } from "../../views/netobserv"
import { netflowPage, querySumSelectors, topologySelectors } from "../../views/netflow-page"
import { dashboard, graphSelector } from "views/dashboards-page"

const metricType = [
    "Bytes",
    "Packets",
    "RTT"
]

const flowRTTPanels = [
    // below 2 panels should appear with the 'node_rtt_seconds' metric
    "top-p50-srtt-per-node-(ms)-chart",
    "top-p99-srtt-per-node-(ms)-chart",
    // below 2 panels should appear with the 'namespace_rtt_seconds' metric
    "top-p50-srtt-per-infra-namespace-(ms)-chart",
    "top-p99-srtt-per-infra-namespace-(ms)-chart",
    // below 2 panels should appear with the 'workload_rtt_seconds' metric
    "top-p50-srtt-per-infra-workload-(ms)-chart",
    "top-p99-srtt-per-infra-workload-(ms)-chart"
]

describe('(OCP-68246 Network_Observability) FlowRTT test', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        Operator.install()
        Operator.createFlowcollector(project, "FlowRTT")
    })

    it("(OCP-68246, aramesha, Network_Observability) Validate flowRTT edge labels and Query Summary stats", function () {
        netflowPage.visit()
        cy.get('#tabs-container li:nth-child(3)').click()
        cy.get('#drawer').should('not.be.empty')

        cy.byTestID("show-view-options-button").should('exist').click().then(views => {
            cy.contains('Display options').should('exist').click()
            // set one display to test with
            cy.byTestID('layout-dropdown').click()
            cy.byTestID('Grid').click()
        })

        cy.byTestID(topologySelectors.metricTypeDrop).should('exist').click()
        cy.get(topologySelectors.metricType).find('li').should('have.length', 3).each((item, index) => {
            cy.wrap(item).should('contain.text', metricType[index])
        })

        cy.get('#TimeFlowRttNs').click()
        cy.byTestID("scope-dropdown").click().get("#host").click()
        cy.contains('Display options').should('exist').click()

        // filter on TCP protocol
        cy.byTestID("column-filter-toggle").click().get('.pf-v5-c-menu__content').should('be.visible')
        cy.get('#group-2-toggle').click().get('#protocol').click()
        cy.byTestID('autocomplete-search').type('TCP' + '{enter}')

        cy.get('[data-test-id=edge-handler]').each((g) => {
            expect(g.text()).to.match(/\d* ms/gm);
        });
        netflowPage.clearAllFilters()

        // verify Query summary panel
        cy.get(querySumSelectors.avgRTT).should('exist').then(avgRTT => {
            cy.checkQuerySummary(avgRTT)
        })
        netflowPage.resetClearFilters()
    })

    it("(OCP-68246, aramesha, Network_Observability) Validate flowRTT dashboards", function () {
        // navigate to 'NetObserv / Main' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("netobserv-main")

        // verify 'TCP latency,p99' panel
        cy.get('[data-test="tcp-latency,-p99-chart"]').find(graphSelector.graphBody).should('not.have.class', 'graph-empty-state')

        cy.checkDashboards(flowRTTPanels)
    })
    after("Delete flowcollector", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})

