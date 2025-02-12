import { Operator, project } from "../../views/netobserv"
import { netflowPage, querySumSelectors, topologySelectors } from "../../views/netflow-page"
import { dashboard } from "views/dashboards-page"

const metricType = [
    "Bytes",
    "Packets",
    "DNS latencies"
]

const DNSPanels = [
    // below 3 panels should appear with the 'node_dns_latency_seconds' metric
    "top-p50-dns-latency-per-node-(ms)-chart",
    "top-p99-dns-latency-per-node-(ms)-chart",
    "dns-error-rate-per-node-chart",
    // below 3 panels should appear with the 'namespace_dns_latency_seconds' metric
    "top-p50-dns-latency-per-infra-namespace-(ms)-chart",
    "top-p99-dns-latency-per-infra-namespace-(ms)-chart",
    "dns-error-rate-per-infra-namespace-chart",
    // below 3 panels should appear with the 'workload_dns_latency_seconds' metric
    "top-p50-dns-latency-per-infra-workload-(ms)-chart",
    "top-p99-dns-latency-per-infra-workload-(ms)-chart",
    "dns-error-rate-per-infra-workload-chart"
]

describe('(OCP-67087 Network_Observability) DNSTracking test', { tags: ['Network_Observability'] }, function () {

    before('any test', function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogin(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))

        Operator.install()
        Operator.createFlowcollector(project, "DNSTracking")
    })

    it("(OCP-67087, aramesha, Network_Observability) Validate DNSLatencies edge label and Query Summary stats", function () {
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
        cy.get('#DnsLatencyMs').click()

        cy.byTestID("scope-dropdown").click().get("#host").click()
        cy.contains('Display options').should('exist').click()

        cy.get('[data-test-id=edge-handler]').should('exist').each((g) => {
            expect(g.text()).to.match(/\d* ms/gm);
        });

        // verify Query Summary stats for DNSTracking
        cy.get(querySumSelectors.dnsAvg).should('exist').then(DNSAvg => {
            cy.checkQuerySummary(DNSAvg)
        })
        netflowPage.resetClearFilters()
    })

    it("(OCP-67087, aramesha, Network_Observability) Validate DNSTracking dashboards", function () {
        // navigate to 'NetObserv / Main' Dashboard page
        dashboard.visit()
        dashboard.visitDashboard("netobserv-main")

        cy.checkDashboards(DNSPanels)
    })

    after("Delete flowcollector and DNS pods", function () {
        Operator.deleteFlowCollector()
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
    })
})
