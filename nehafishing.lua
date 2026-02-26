ADDON:ImportObject(OBJECT_TYPE.TEXT_STYLE)
ADDON:ImportObject(OBJECT_TYPE.BUTTON)
ADDON:ImportObject(OBJECT_TYPE.DRAWABLE)
ADDON:ImportObject(OBJECT_TYPE.NINE_PART_DRAWABLE)
ADDON:ImportObject(OBJECT_TYPE.COLOR_DRAWABLE)
ADDON:ImportObject(OBJECT_TYPE.WINDOW)
ADDON:ImportObject(OBJECT_TYPE.LABEL)
ADDON:ImportObject(OBJECT_TYPE.ICON_DRAWABLE)
ADDON:ImportObject(OBJECT_TYPE.IMAGE_DRAWABLE)

ADDON:ImportAPI(API_TYPE.CHAT.id)
ADDON:ImportAPI(API_TYPE.UNIT.id)

local labelAnchor = CreateEmptyWindow("labelAnchor", "UIParent")
labelAnchor:Show(true)
labelAnchor:Enable(true)
local lblDuration = labelAnchor:CreateChildWidget("label", "lblDuration", 0, true)
lblDuration:Show(true)
lblDuration:EnablePick(false)
lblDuration.style:SetColor(1, 1, 1, 1.0)
lblDuration.style:SetFontSize(15)
lblDuration.style:SetOutline(true)
lblDuration.style:SetAlign(ALIGN_LEFT)
lblDuration:AddAnchor("LEFT",labelAnchor,0,0)
lblDuration:SetText("")
local skipIter = 1
local updateFrequency = 1



function labelAnchor:OnUpdate(dt)

    local nScrX_Tar, nScrY_Tar, nScrZ_Tar = X2Unit:GetUnitScreenPosition("target")
    if nScrX_Tar == nil or nScrY_Tar == nil or nScrZ_Tar == nil then
        labelAnchor:AddAnchor("TOPLEFT", "UIParent", 5000, 5000) 
    elseif nScrZ_Tar > 0 then
        local x = math.floor(0.5+nScrX_Tar)
        local y = math.floor(0.5+nScrY_Tar)
        labelAnchor:AddAnchor("TOPLEFT", "UIParent", x+40, y-40)
        if skipIter < updateFrequency then
            skipIter = skipIter + 1
            return
        end
        skipIter = 1
        local fishSize = ''
        local unitHiddenBuffCount = X2Unit:UnitHiddenBuffCount("target")
        for i=1, unitHiddenBuffCount do
            local unitHiddenBuffTooltip = X2Unit:UnitHiddenBuffTooltip("target", i)
            if unitHiddenBuffTooltip then
                for k, v in pairs(unitHiddenBuffTooltip) do
                    if k == "name" then
                        if string.find(v, "Minutes") then
                            fishSize = tostring(v)
                        end
                    end
                end
            else
            end
        end
       lblDuration.style:SetColor(0, 1, 0, 1.0)
       lblDuration:SetText(tostring(fishSize))
    end
end

labelAnchor:SetHandler("OnUpdate", labelAnchor.OnUpdate)