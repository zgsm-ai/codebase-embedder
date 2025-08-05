import requests
import json

url = "http://127.0.0.1:8888/codebase-embedder/api/v1/codebase/tree"

payload = json.dumps({
  "clientId": "5",
  "codebasePath": "D:\\workspace",
  "codebaseName": "codebase-embedder",
  "maxDepth": 3,
  "includeFiles": True
})
headers = {
  'Authorization': 'Bearer aee59212-46c5-4726-807a-cb9121c2ab5f&code=5650566a-626c-4fcb-a490-f3f3099b7105.aee59212-46c5-4726-807a-cb9121c2ab5f.6aa578f3-e98d-40b7-bbdd-c344bc4861e0',
  'Content-Type': 'application/json'
}

response = requests.request("POST", url, headers=headers, data=payload)

print(response.text)