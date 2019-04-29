// +build !nodbxml

package main

import (
	"fmt"
)

type MetadataCountsResponse map[string]map[string]int

// $router->map('POST', '/metadata_counts', function () {
//     $input = file_get_contents('php://input');
//     $data = json_decode($input, true);
//     $corpus = $data['corpus'];
//     $components = $data['components'];
//     $xpath = $data['xpath'];

//     $counts = get_metadata_counts($corpus, $components, $xpath);
//     header('Content-Type: application/json');
//     echo json_encode($counts);
// });

// function get_metadata_counts($corpus, $components, $xpath)
// {
//     global $dbuser, $dbpwd;

//     if ($corpus == 'sonar') {
//         $serverInfo = getServerInfo($corpus, $components[0]);
//     } else {
//         $serverInfo = getServerInfo($corpus, false);
//     }

//     $databases = corpusToDatabase($components, $corpus);

//     $dbhost = $serverInfo['machine'];
//     $dbport = $serverInfo['port'];
//     $session = new Session($dbhost, $dbport, $dbuser, $dbpwd);

//     $result = array();
//     foreach ($databases as $database) {
//         $xquery = '{
//             for $n
//             in (
//                 for $node
//                 in db:open("'.$database.'")'.$xpath.'
//                 return $node/ancestor::alpino_ds/metadata/meta)
//             let $k := $n/@name
//             let $t := $n/@type
//             group by $k, $t
//             order by $k, $t

//             return element meta {
//                 attribute name {$k},
//                 attribute type {$t},
//                 for $m in $n
//                 let $v := $m/@value
//                 group by $v
//                 return element count {
//                     attribute value {$v}, count($m)
//                 }
//             }
//         }';

//         $m_query = '<metadata>'.$xquery.'</metadata>';

//         $query = $session->query($m_query);
//         $result[$database] = $query->execute();
//         $query->close();
//     }
//     $session->close();

//     // Combine the XMLs into an array with total counts over all databases
//     $totals = array();
//     foreach ($result as $db => $m) {
//         $xml = new SimpleXMLElement($m);
//         foreach ($xml as $group => $counts) {
//             $name = (string) $counts['name'];
//             $a2 = array();
//             foreach ($counts as $k => $v) {
//                 $a2[(string) $v['value']] = (int) $v;
//             }
//             if (isset($totals[$name])) {
//                 $a1 = $totals[$name];
//                 $sums = array();
//                 foreach (array_keys($a1 + $a2) as $key) {
//                     $sums[$key] = (isset($a1[$key]) ? $a1[$key] : 0) + (isset($a2[$key]) ? $a2[$key] : 0);
//                 }
//                 $totals[$name] = $sums;
//             } else {
//                 $totals[$name] = $a2;
//             }
//         }
//     }

//     return $totals;
// }

func api_gretel_metadata_counts(q *Context) {
	q.w.Header().Set("Access-Control-Allow-Origin", "*")

	q.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	q.w.Header().Set("Cache-Control", "no-cache")
	q.w.Header().Add("Pragma", "no-cache")

	fmt.Fprint(q.w, "[]") // Metadata is unsupported for now
}
