package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func registerSupplierRoutes(api *gin.RouterGroup) {
	api.GET("/suppliers/choices", middleware.AdminAuth(), middleware.RequirePermission(authz.ChannelSensitiveWrite), controller.GetSupplierChoices)
	suppliers := api.Group("/suppliers", middleware.RootAuth(), middleware.DisableCache())
	{
		suppliers.GET("", controller.GetSuppliers)
		suppliers.POST("", controller.CreateSupplier)
		suppliers.PUT("/:id", controller.UpdateSupplier)
		suppliers.DELETE("/:id", controller.DeleteSupplier)
		suppliers.GET("/:id/profiles", controller.GetSupplierProfiles)
		suppliers.POST("/:id/profiles", controller.CreateSupplierProfile)
		suppliers.PUT("/profiles/:id", controller.UpdateSupplierProfile)
		suppliers.DELETE("/profiles/:id", controller.DeleteSupplierProfile)
		suppliers.GET("/profiles/:id/tariffs", controller.GetSupplierPricingTariffs)
		suppliers.POST("/profiles/:id/tariffs", controller.PublishSupplierPricingTariffs)
		suppliers.POST("/profiles/:id/sync", controller.SyncSupplierPricingProfile)
		suppliers.GET("/group-bindings", controller.GetSupplierGroupBindings)
		suppliers.PUT("/group-bindings", controller.UpdateSupplierGroupBindings)
		suppliers.GET("/migration-preview", controller.GetSupplierMigrationPreview)
	}
	api.GET("/official-prices", middleware.RootAuth(), middleware.DisableCache(), controller.GetOfficialPrices)
	api.PUT("/official-prices", middleware.RootAuth(), middleware.DisableCache(), controller.UpdateOfficialPrices)
	api.DELETE("/official-prices", middleware.RootAuth(), middleware.DisableCache(), controller.DeleteOfficialPrice)
}
