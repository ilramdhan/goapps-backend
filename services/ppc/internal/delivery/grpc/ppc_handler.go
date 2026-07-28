// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	bfsapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/balanceforsale"
	capacityapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/capacity"
	changeoverapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/changeover"
	commonlotapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/commonlot"
	customerapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/customer"
	dpapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dailyperf"
	dashapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/dashboard"
	demandapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	downtimereasonapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/downtimereason"
	etlapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/etl"
	lookupapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/lookup"
	lotapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/lot"
	machineapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/machine"
	machinegroupapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/machinegroup"
	"github.com/mutugading/goapps-backend/services/ppc/internal/application/machinesync"
	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	productconfigapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/productconfig"
	pmpapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/productmachineparameter"
	shiftapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/shift"
	thresholdapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/threshold"
	wastecategoryapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/wastecategory"
	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	dpdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
)

// unimplementedBase nests the generated Unimplemented server one embedding level
// below the concrete sub-handlers. Go selects the shallowest promoted method, so
// implemented master RPCs (depth 1 via sub-handlers) win while every not-yet-built
// RPC falls through to Unimplemented (depth 2) returning codes.Unimplemented.
type unimplementedBase struct {
	ppcv1.UnimplementedPPCServiceServer
}

// PPCHandler is the composite gRPC handler for the single PPCService. It embeds
// one focused sub-handler per master domain; unbuilt RPCs remain Unimplemented.
type PPCHandler struct {
	unimplementedBase
	*machineGroupHandler
	*machineHandler
	*customerHandler
	*lotHandler
	*productConfigHandler
	*capacityHandler
	*productMachineParameterHandler
	*thresholdHandler
	*downtimeReasonHandler
	*wasteCategoryHandler
	*demandHandler
	*planItemHandler
	*workOrderHandler
	*suggestHandler
	*dailyPerfHandler
	*changeoverHandler
	*dashboardHandler
	*packingHandler
	*lookupHandler
	*shiftHandler
}

// Deps carries the application services required to build the PPC handler.
type Deps struct {
	MachineGroup            *machinegroupapp.Service
	Machine                 *machineapp.Service
	MachineSync             *machinesync.Usecase
	Customer                *customerapp.Service
	Lot                     *lotapp.Service
	ProductConfig           *productconfigapp.Service
	Capacity                *capacityapp.Service
	ProductMachineParameter *pmpapp.Service
	Threshold               *thresholdapp.Service
	DowntimeReason          *downtimereasonapp.Service
	WasteCategory           *wastecategoryapp.Service
	Demand                  *demandapp.Service
	PlanItem                *planitemapp.Service
	WorkOrder               *workorderapp.Service
	Suggest                 *etlapp.SuggestService
	DailyPerf               *dpapp.Service
	Changeover              *changeoverapp.Service
	Dashboard               *bfsapp.Service
	DashboardRead           *dashapp.Service
	SnapshotReader          dpdomain.EfficiencySnapshotReader
	GradeActuals            *etlapp.GradeActualService
	CommonLot               *commonlotapp.Service
	DailyPerfExporter       *dpapp.Exporter
	Lookup                  *lookupapp.Service
	Shift                   *shiftapp.Service
}

// NewPPCHandler wires the composite PPC service handler from its dependencies.
func NewPPCHandler(deps Deps) *PPCHandler {
	return &PPCHandler{
		machineGroupHandler:            newMachineGroupHandler(deps.MachineGroup),
		machineHandler:                 newMachineHandler(deps.Machine, deps.MachineSync),
		customerHandler:                newCustomerHandler(deps.Customer),
		lotHandler:                     newLotHandler(deps.Lot),
		productConfigHandler:           newProductConfigHandler(deps.ProductConfig),
		capacityHandler:                newCapacityHandler(deps.Capacity),
		productMachineParameterHandler: newProductMachineParameterHandler(deps.ProductMachineParameter),
		thresholdHandler:               newThresholdHandler(deps.Threshold),
		downtimeReasonHandler:          newDowntimeReasonHandler(deps.DowntimeReason),
		wasteCategoryHandler:           newWasteCategoryHandler(deps.WasteCategory),
		demandHandler:                  newDemandHandler(deps.Demand),
		planItemHandler:                newPlanItemHandler(deps.PlanItem),
		workOrderHandler:               newWorkOrderHandler(deps.WorkOrder, deps.PlanItem),
		suggestHandler:                 newSuggestHandler(deps.Suggest),
		dailyPerfHandler:               newDailyPerfHandler(deps.DailyPerf),
		changeoverHandler:              newChangeoverHandler(deps.Changeover),
		dashboardHandler:               newDashboardHandler(deps.Dashboard, deps.DashboardRead, deps.SnapshotReader),
		packingHandler:                 newPackingHandler(deps.GradeActuals, deps.CommonLot, deps.DailyPerfExporter),
		lookupHandler:                  newLookupHandler(deps.Lookup),
		shiftHandler:                   newShiftHandler(deps.Shift),
	}
}

// Compile-time assertion that PPCHandler satisfies the generated server API.
var _ ppcv1.PPCServiceServer = (*PPCHandler)(nil)
