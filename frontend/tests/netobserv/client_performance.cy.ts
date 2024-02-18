import { netflowPage, loadTimes, memoryUsage, overviewSelectors } from "../../views/netflow-page"
import { Operator, project } from "../../views/netobserv"
import { catalogSources } from "../../views/catalog-source"

function getTopologyScopeURL(scope: string): string {
    return `**/flow/metrics?filters=&limit=50&recordType=flowLog&dedup=true&packetLoss=all&timeRange=300&rateInterval=30s&step=15s&type=bytes&aggregateBy=${scope}`
}

describe("(OCP-67725, memodi) Network_Observability Client Performances", { browser: 'chrome', tags: ['Network_Observability'] }, function () {
    before("tests", function () {
        cy.adminCLI(`oc adm policy add-cluster-role-to-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.login(Cypress.env('LOGIN_IDP'), Cypress.env('LOGIN_USERNAME'), Cypress.env('LOGIN_PASSWORD'))
        cy.switchPerspective('Administrator');

        // specify --env noo_release=upstream to run tests 
        // from most recent "main" image
        let catalogImg
        let catalogDisplayName = "Production Operators"
        const catSrc = Cypress.env('noo_catalog_src')
        if (catSrc == "upstream") {
            catalogImg = 'quay.io/netobserv/network-observability-operator-catalog:v0.0.0-main'
            this.catalogSource = "netobserv-test"
            catalogDisplayName = "NetObserv QE"
            catalogSources.createCustomCatalog(catalogImg, this.catalogSource, catalogDisplayName)
        }
        else {
            catalogSources.enableQECatalogSource(this.catalogSource, catalogDisplayName)
        }
        Operator.install(catalogDisplayName)
        Operator.createFlowcollector(project)
    })

    beforeEach("test", function () {
        cy.intercept('**/backend/api/loki/flow/metrics*').as('call1')
        cy.visit('/netflow-traffic')
        // wait for all calls to complete before checking due to bug
        cy.wait('@call1', { timeout: 60000 }).wait('@call1')

    })

    it("(OCP-67725, memodi, Network_Observability) should measure overview page load times", function () {
        netflowPage.clearAllFilters()
        const start = performance.now()
        cy.intercept('GET', getTopologyScopeURL("namespace"), {
            fixture: 'netobserv/overview_perf_ns.json'
        })
        cy.intercept('GET', getTopologyScopeURL("app"), {
            fixture: 'netobserv/overview_perf_app.json'
        })

        cy.get('#overview-flex').contains(overviewSelectors.defaultPanels[0]).should('be.visible').then(() => {
            cy.wrap(performance.now()).then(end => {
                let pageload = Math.round(end - start)
                let curMemoryUsage = Math.round(window.performance.memory.usedJSHeapSize / 1048576)
                cy.log(`Overview page load took ${pageload} ms.`)
                cy.log(`Overview page memory consumption ${curMemoryUsage} MB`)
                let thresPageload = loadTimes.overview + loadTimes.overview * 0.5
                let memThreshold = memoryUsage.overview + memoryUsage.overview * 0.5
                expect(pageload).to.be.lessThan(thresPageload)
                expect(curMemoryUsage).to.be.lessThan(memThreshold)
            })
        })
    })

    it("(OCP-67725, memodi, Network_Observability) should measure table page load times", function () {
        cy.get('#tabs-container li:nth-child(2)').click()
        netflowPage.clearAllFilters()
        const start = performance.now()
        const url = '**/backend/api/loki/flow/metrics*'
        cy.intercept('GET', url, {
            fixture: 'netobserv/netflow_table_perf.json'
        })
        cy.byTestID("table-composable").should('be.visible').then(() => {
            cy.wrap(performance.now()).then(end => {
                let pageload = Math.round(end - start)
                let curMemoryUsage = Math.round(window.performance.memory.usedJSHeapSize / 1048576)
                cy.log(`Table view page load took ${pageload} ms.`)
                cy.log(`Table view memory consumption ${curMemoryUsage} MB`)
                let thresPageload = loadTimes.table + loadTimes.table * 0.5
                let memThreshold = memoryUsage.table + memoryUsage.table * 0.5
                expect(pageload).to.be.lessThan(thresPageload)
                expect(curMemoryUsage).to.be.lessThan(memThreshold)
            })
        })
    })

    it("(OCP-67725, memodi, Network_Observability) should measure topology page load times", function () {
        cy.get('#tabs-container li:nth-child(3)').click()
        netflowPage.clearAllFilters()
        const start = performance.now()
        cy.intercept('GET', getTopologyScopeURL("namespace"), {
            fixture: 'netobserv/flow_metrics_perf.json'
        })
        cy.get('[data-surface="true"]').should('be.visible').then(() => {
            cy.wrap(performance.now()).then(end => {
                let pageload = Math.round(end - start)
                let curMemoryUsage = Math.round(window.performance.memory.usedJSHeapSize / 1048576)
                cy.log(`Topology view page load took ${pageload} ms.`)
                cy.log(`Topology view memory consumption ${curMemoryUsage} MB`)
                let thresPageload = loadTimes.topology + loadTimes.topology * 0.5
                let memThreshold = memoryUsage.topology + memoryUsage.topology * 0.5
                expect(pageload).to.be.lessThan(thresPageload)
                expect(curMemoryUsage).to.be.lessThan(memThreshold)
            })
        })
    })
    afterEach("test", function () {
        netflowPage.resetClearFilters()
    })

    after("suite", function () {
        cy.adminCLI(`oc adm policy remove-cluster-role-from-user cluster-admin ${Cypress.env('LOGIN_USERNAME')}`)
        cy.uiLogout()
    })
})
