-- name: GetDonorAdjustmentsByPerson :many
SELECT *
FROM donor_adjustments
WHERE person_id = ?1; 

-- name: SaveDonorAdjustment :one
INSERT INTO donor_adjustments(
    person_id, 
    display_name, 
    slug,
    amount 
) 
VALUES (
    ?1, 
    ?2, 
    ?3,
    ?4
) 
ON CONFLICT(person_id, slug) DO 
UPDATE SET display_name = ?2, 
           amount = ?4
WHERE person_id = ?1 AND slug = ?3
RETURNING *;